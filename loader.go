package envconfig

import (
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
	emptyVal   = ""
	trueVal    = "true"
	defaultSep = ","
	maskedVal  = "[*****]"

	envTag       = "env"
	defaultTag   = "default"
	requiredTag  = "required"
	sensitiveTag = "sensitive"
	sepTag       = "sep"
)

// Pre-computed types to avoid repeated reflect.TypeOf calls at runtime
var (
	typeDuration = reflect.TypeOf(time.Duration(0))
	typeString   = reflect.TypeOf("")
	typeInt      = reflect.TypeOf(0)
	typeInt64    = reflect.TypeOf(int64(0))
	typeFloat32  = reflect.TypeOf(float32(0))
	typeFloat64  = reflect.TypeOf(float64(0))
	typeBool     = reflect.TypeOf(false)
)

func Load[T any]() (T, error) {
	var cfg T
	var errs []error

	v := reflect.ValueOf(&cfg).Elem()
	t := v.Type()

	if t.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("config must be a struct")
	}

	for i := range v.NumField() {
		field := v.Field(i)
		fieldType := t.Field(i)

		envKey := fieldType.Tag.Get(envTag)
		if envKey == emptyVal {
			continue
		}

		required := fieldType.Tag.Get(requiredTag) == trueVal
		defValue := fieldType.Tag.Get(defaultTag)
		sep := fieldType.Tag.Get(sepTag)
		if sep == emptyVal {
			sep = defaultSep
		}

		rawValue, exists := os.LookupEnv(envKey)

		var value string
		if exists && rawValue != emptyVal {
			value = rawValue
		} else {
			if required {
				// Collect the error and continue to the next field.
				errs = append(errs, fmt.Errorf("missing required env var: %s", envKey))
				continue
			}
			value = defValue
		}

		if err := setFieldValue(field, value, envKey, sep); err != nil {
			errs = append(errs, err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func setFieldValue(field reflect.Value, value string, envKey string, sep string) error {
	// Handle slice types before everything else.
	if field.Kind() == reflect.Slice {
		return setSliceValue(field, value, envKey, sep)
	}

	// Skip unset optional scalar fields to avoid parse errors on empty strings.
	if value == emptyVal {
		return nil
	}

	// If the field (or a pointer to it) implements Unmarshaler, delegate entirely.
	// field.Addr() is safe here because Load() always works on &cfg.Elem(),
	// which guarantees all fields are addressable.
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(Unmarshaler); ok {
			if err := u.UnmarshalEnv(value); err != nil {
				return fmt.Errorf("invalid value for %s: %w", envKey, err)
			}
			return nil
		}
	}

	switch field.Type() {
	case typeDuration:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for %s: %w", envKey, err)
		}
		field.SetInt(int64(parsed))

	case typeString:
		field.SetString(value)

	case typeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid int for %s: %w", envKey, err)
		}
		field.SetInt(int64(parsed))

	case typeInt64:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int64 for %s: %w", envKey, err)
		}
		field.SetInt(parsed)

	case typeFloat32:
		parsed, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return fmt.Errorf("invalid float32 for %s: %w", envKey, err)
		}
		field.SetFloat(parsed)

	case typeFloat64:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float64 for %s: %w", envKey, err)
		}
		field.SetFloat(parsed)

	case typeBool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool for %s: %w", envKey, err)
		}
		field.SetBool(parsed)

	default:
		return fmt.Errorf("unsupported type %s for env %s", field.Type(), envKey)
	}

	return nil
}

func setSliceValue(field reflect.Value, value string, envKey string, sep string) error {
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

		if err := setFieldValue(elem, part, fmt.Sprintf("%s[%d]", envKey, i), sep); err != nil {
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
