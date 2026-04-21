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
	sepTag       = "comma"
	kvSepTag     = "kvSep"
	prefixTag    = "envPrefix"
)

// Pre-computed types to avoid repeated reflect.TypeOf calls at runtime
var (
	typeDuration = reflect.TypeOf(time.Duration(0))
	typeTime     = reflect.TypeOf(time.Time{})
	typeString   = reflect.TypeOf("")

	// Signed Types
	typeInt   = reflect.TypeOf(0)
	typeInt32 = reflect.TypeOf(int32(0)) // valid also for `rune`
	typeInt64 = reflect.TypeOf(int64(0))

	// Unsigned Types
	typeUint   = reflect.TypeOf(uint(0))
	typeUint8  = reflect.TypeOf(uint8(0)) // valid also for `byte`
	typeUint64 = reflect.TypeOf(uint64(0))

	// Float
	typeFloat32 = reflect.TypeOf(float32(0))
	typeFloat64 = reflect.TypeOf(float64(0))

	typeBool = reflect.TypeOf(false)
)

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

		// Nested struct (excluding special types)
		if field.Kind() == reflect.Struct && field.Type() != typeDuration && field.Type() != typeTime {
			newPath := joinPath(path, fieldType.Name)
			if err := processStruct(field, fieldPrefix, newPath); err != nil {
				errs = append(errs, err)
			}
			continue
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

		if err := setFieldValue(field, value, currentPath, fullEnvKey, sep, kvSep); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ---------------------- VALUE SETTERS ----------------------

func setFieldValue(field reflect.Value, value string, path string, envKey string, sep string, kvSep string) error {
	// Handle pointer types: allocate, recurse on the pointed-to value, then assign.
	if field.Kind() == reflect.Pointer {
		// An empty value leaves a nil pointer (not a pointer to a zero value).
		if value == emptyVal {
			return nil
		}
		ptr := reflect.New(field.Type().Elem())
		if err := setFieldValue(ptr.Elem(), value, path, envKey, sep, kvSep); err != nil {
			return err
		}
		field.Set(ptr)
		return nil
	}

	// Map
	if field.Kind() == reflect.Map {
		return setMapValue(field, value, path, envKey, sep, kvSep)
	}

	// Slice
	if field.Kind() == reflect.Slice {
		return setSliceValue(field, value, path, envKey, sep, kvSep)
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

	switch field.Type() {
	case typeDuration:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: invalid duration for %s: %w", path, envKey, err)
		}
		field.SetInt(int64(parsed))

	case typeTime:
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return fmt.Errorf("%s: invalid time for %s (RFC3339): %w", path, envKey, err)
		}
		field.Set(reflect.ValueOf(parsed))

	case typeString:
		field.SetString(value)

	case typeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s: invalid int for %s: %w", path, envKey, err)
		}
		field.SetInt(int64(parsed))

	case typeInt32: // it works for int32 and alias
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%s: invalid int32/rune for %s: %w", path, envKey, err)
		}
		field.SetInt(parsed)

	case typeInt64:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid int64 for %s: %w", path, envKey, err)
		}
		field.SetInt(parsed)

	case typeUint:
		// Passing 0 as bitSize, the parser understands whether the platform uses 32-bit or 64-bit uints.
		parsed, err := strconv.ParseUint(value, 10, 0)
		if err != nil {
			return fmt.Errorf("%s: invalid uint for %s: %w", path, envKey, err)
		}
		field.SetUint(parsed)

	case typeUint8: // It works for uint8 e byte
		parsed, err := strconv.ParseUint(value, 10, 8)
		if err != nil {
			return fmt.Errorf("%s: invalid uint8/byte for %s: %w", path, envKey, err)
		}
		field.SetUint(parsed)

	case typeUint64:
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid uint64 for %s: %w", path, envKey, err)
		}
		field.SetUint(parsed)

	case typeFloat32:
		parsed, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return fmt.Errorf("%s: invalid float32 for %s: %w", path, envKey, err)
		}
		field.SetFloat(parsed)

	case typeFloat64:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid float64 for %s: %w", path, envKey, err)
		}
		field.SetFloat(parsed)

	case typeBool:
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

func setSliceValue(field reflect.Value, value string, path string, envKey string, sep string, kvSep string) error {
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

		if err := setFieldValue(elem, part, elemPath, envKey, sep, kvSep); err != nil {
			errs = append(errs, err)
			continue
		}

		slice.Index(i).Set(elem)
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	field.Set(slice)
	return nil
}

func setMapValue(field reflect.Value, value string, path string, envKey string, sep string, kvSep string) error {
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

		if err := setFieldValue(elem, mapVal, elemPath, envKey, sep, kvSep); err != nil {
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

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}
