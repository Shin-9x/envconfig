// Package envconfig provides a generic, reflection-based loader for populating
// configuration structs from environment variables.
//
// The entry point is [Load], which reads struct field tags to determine which
// environment variables to look up, how to parse them, and how to validate the
// resulting values.
package envconfig

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Constants
const (
	emptyVal     = ""
	trueVal      = "true"
	defaultSep   = ","
	defaultKvSep = ":"
	maskedVal    = "[*****]"

	envTag       = "env"
	defaultTag   = "default"
	requiredTag  = "required"
	sensitiveTag = "sensitive"
	sepTag       = "sep"
	kvSepTag     = "kvSep"
	prefixTag    = "envPrefix"
)

// Pre-computed types to avoid repeated reflect.TypeOf calls at runtime
var (
	typeDuration = reflect.TypeOf(time.Duration(0))
	typeTime     = reflect.TypeOf(time.Time{})
)

// Load reads environment variables into a new value of type T and returns it.
// T must be a struct type; any other kind causes an immediate error.
//
// Field mapping is controlled entirely by struct tags (see package-level tag
// constants). Nested structs are traversed recursively. Fields that implement
// [Unmarshaler] or [encoding.TextUnmarshaler] are delegated to their own
// unmarshalling logic.
//
// All field errors are collected and returned together as a single joined error,
// so callers receive a complete picture of every misconfigured variable in one
// pass.
func Load[T any]() (T, error) {
	var cfg T

	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("config must be a struct")
	}

	err := processStruct(v, "", "")
	if err != nil {
		return cfg, err
	}

	return cfg, nil
}

// ---------------------- CORE ----------------------

// processStruct iterates over the exported fields of v, resolving each field's
// environment variable according to the tags present on the field.
//
// prefix is prepended to every env key resolved within this struct; path is a
// dot-separated string used solely for human-readable error messages.
//
// Nested structs (excluding time.Duration, time.Time, and types that implement
// a recognized unmarshalling interface) are processed recursively.
func processStruct(v reflect.Value, prefix string, path string) error {
	t := v.Type()
	var errs []error

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported
		if !field.CanSet() {
			continue
		}

		// Prefix handling
		fieldPrefix := prefix
		if p := fieldType.Tag.Get(prefixTag); p != emptyVal {
			fieldPrefix += p
		}

		// Nested struct (excluding special types and types that handle themselves)
		f := field
		if f.Kind() == reflect.Pointer && !f.IsNil() {
			f = f.Elem()
		}

		if f.Kind() == reflect.Struct && f.Type() != typeDuration && f.Type() != typeTime {
			isCustomScalar := false
			if field.CanAddr() {
				addr := field.Addr().Interface()
				_, isCustomScalar = addr.(Unmarshaler)
				if !isCustomScalar {
					_, isCustomScalar = addr.(interface{ UnmarshalText([]byte) error })
				}
			}

			if !isCustomScalar {
				newPath := joinPath(path, fieldType.Name)
				if err := processStruct(f, fieldPrefix, newPath); err != nil {
					errs = append(errs, err)
				}
				continue
			}
		}

		envKey := fieldType.Tag.Get(envTag)
		if envKey == emptyVal {
			continue
		}

		fullEnvKey := fieldPrefix + envKey
		currentPath := joinPath(path, fieldType.Name)

		required := fieldType.Tag.Get(requiredTag) != emptyVal
		defValue := fieldType.Tag.Get(defaultTag)

		sep := fieldType.Tag.Get(sepTag)
		if sep == emptyVal {
			sep = defaultSep
		}

		kvSep := fieldType.Tag.Get(kvSepTag)
		if kvSep == emptyVal {
			kvSep = defaultKvSep
		}

		// Collect the validate spec once per field
		validateSpec := fieldType.Tag.Get(validateTag)

		rawValue, exists := os.LookupEnv(fullEnvKey)

		var value string
		if exists && rawValue != emptyVal {
			value = strings.TrimSpace(rawValue)
		} else {
			if required {
				errs = append(errs, fmt.Errorf("%s: missing required env var %s", currentPath, fullEnvKey))
				continue
			}
			value = defValue
		}

		if err := setFieldValue(field, value, currentPath, fullEnvKey, sep, kvSep, validateSpec); err != nil {
			errs = append(errs, err)
		} else if value != emptyVal {
			// Only validate after a successful set and only when there is an actual value.
			if err := validateFieldValue(field, value, currentPath, fullEnvKey, validateSpec); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// ---------------------- VALUE SETTERS ----------------------

// setFieldValue writes value into field, dispatching on the field's kind.
//
// Pointer fields are allocated on demand; an empty value leaves a nil pointer
// rather than a pointer to a zero value. Slices and maps are handled by their
// dedicated helpers. Fields that implement [Unmarshaler] or
// [encoding.TextUnmarshaler] are delegated to those interfaces before any
// kind-based switch is reached. time.Duration is treated as a special case and
// parsed with [time.ParseDuration].
func setFieldValue(field reflect.Value, value, path, envKey, sep, kvSep, validateSpec string) error {
	// Handle pointer types: allocate, recurse on the pointed-to value, then assign.
	if field.Kind() == reflect.Pointer {
		// An empty value leaves a nil pointer (not a pointer to a zero value).
		if value == emptyVal {
			return nil
		}
		ptr := reflect.New(field.Type().Elem())
		if err := setFieldValue(ptr.Elem(), value, path, envKey, sep, kvSep, validateSpec); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}

	// Map
	if field.Kind() == reflect.Map {
		return setMapValue(field, value, path, envKey, sep, kvSep, validateSpec)
	}

	// Slice
	if field.Kind() == reflect.Slice {
		return setSliceValue(field, value, path, envKey, sep, kvSep, validateSpec)
	}

	// Skip unset optional scalar fields to avoid parse errors on empty strings.
	if value == emptyVal {
		return nil
	}

	// If the field (or a pointer to it) implements Unmarshaler, delegate entirely.
	// field.Addr() is safe here because Load() always works on &cfg.Elem(),
	// which guarantees all fields are addressable.
	if field.CanAddr() {
		// Custom envconfig.Unmarshaler interface control
		if u, ok := field.Addr().Interface().(Unmarshaler); ok {
			if err := u.UnmarshalEnv(value); err != nil {
				return fmt.Errorf("%s: invalid value for %s: %w", path, envKey, err)
			}
			return nil
		}

		// Standard encoding.TextUnmarshaler interface control
		if tu, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			// UnmarshalText requires a byte slice
			if err := tu.UnmarshalText([]byte(value)); err != nil {
				return fmt.Errorf("%s: invalid text for %s: %w", path, envKey, err)
			}
			return nil
		}
	}

	if field.Type() == typeDuration {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: invalid duration for %s: %w", path, envKey, err)
		}
		field.SetInt(int64(parsed))
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int:
		parsed, err := strconv.ParseInt(value, 10, 0)
		if err != nil {
			return fmt.Errorf("%s: invalid int for %s: %w", path, envKey, err)
		}
		field.SetInt(parsed)

	case reflect.Int32:
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%s: invalid int32 for %s: %w", path, envKey, err)
		}
		field.SetInt(parsed)

	case reflect.Int64:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid int64 for %s: %w", path, envKey, err)
		}
		field.SetInt(parsed)

	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid uint for %s: %w", path, envKey, err)
		}
		field.SetUint(parsed)

	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid float for %s: %w", path, envKey, err)
		}
		field.SetFloat(parsed)

	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s: invalid bool for %s: %w", path, envKey, err)
		}
		field.SetBool(parsed)

	default:
		return fmt.Errorf("%s: unsupported type %s for env %s", path, field.Type(), envKey)
	}

	return nil
}

// setSliceValue splits value on sep and populates field with the resulting
// elements. Each element is parsed through [setFieldValue] and then validated.
// An empty value sets field to a nil slice rather than a single-element slice
// containing an empty string. Errors from individual elements are collected and
// returned together.
func setSliceValue(field reflect.Value, value, path, envKey, sep, kvSep, validateSpec string) error {
	// An empty value produces a nil slice, not [""]
	if value == emptyVal {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}

	parts := strings.Split(value, sep)
	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), len(parts), len(parts))

	var errs []error
	for i, part := range parts {
		part = strings.TrimSpace(part)
		elem := reflect.New(elemType).Elem()

		elemPath := fmt.Sprintf("%s[%d]", path, i)

		if err := setFieldValue(elem, part, elemPath, envKey, sep, kvSep, validateSpec); err != nil {
			errs = append(errs, err)
			continue
		}

		if part != emptyVal {
			if err := validateFieldValue(elem, part, elemPath, envKey, validateSpec); err != nil {
				errs = append(errs, err)
				continue
			}
		}

		slice.Index(i).Set(elem)
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	field.Set(slice)
	return nil
}

// setMapValue splits value on sep, then splits each token on kvSep to obtain
// key-value pairs, and populates field accordingly. Only string map keys are
// supported. An empty value initializes field to an empty (non-nil) map.
// Errors from individual entries are collected and returned together.
func setMapValue(field reflect.Value, value, path, envKey, sep, kvSep, validateSpec string) error {
	// An empty value produces an empty map
	if value == emptyVal {
		field.Set(reflect.MakeMap(field.Type()))
		return nil
	}

	// Enforce string keys for simplicity and common use cases
	if field.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("%s: unsupported map key type %s for env %s (only string keys are supported)", path, field.Type().Key(), envKey)
	}

	parts := strings.Split(value, sep)
	elemType := field.Type().Elem()
	m := reflect.MakeMap(field.Type())

	var errs []error
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == emptyVal {
			continue
		}

		kv := strings.SplitN(part, kvSep, 2)
		if len(kv) != 2 {
			errs = append(errs, fmt.Errorf("%s: malformed map item %q for env %s (expected key%svalue)", path, part, envKey, kvSep))
			continue
		}

		mapKey := strings.TrimSpace(kv[0])
		mapVal := strings.TrimSpace(kv[1])

		elem := reflect.New(elemType).Elem()
		elemPath := fmt.Sprintf("%s[%s]", path, mapKey)

		if err := setFieldValue(elem, mapVal, elemPath, envKey, sep, kvSep, validateSpec); err != nil {
			errs = append(errs, err)
			continue
		}

		if err := validateFieldValue(elem, mapVal, elemPath, envKey, validateSpec); err != nil {
			errs = append(errs, err)
			continue
		}

		m.SetMapIndex(reflect.ValueOf(mapKey), elem)
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	field.Set(m)
	return nil
}

// ---------------------- UTILS ----------------------

// joinPath returns a dot-separated path string for use in error messages.
// If parent is empty, child is returned unchanged.
func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}
