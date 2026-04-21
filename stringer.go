package envconfig

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
)

const comma = ","
const space = " "
const commaAndSpace = comma + space

// Mask returns a string representation of any struct,
// masking fields tagged as sensitive:"true".
func Mask(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "<nil>"
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return fmt.Sprintf("%v", v)
	}

	parts := maskStruct(rv, "", "")

	return fmt.Sprintf("%s: {%s}", rv.Type().Name(), strings.Join(parts, commaAndSpace))
}

// ---------------------- CORE ----------------------

func maskStruct(v reflect.Value, prefix string, path string) []string {
	t := v.Type()
	var parts []string

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		if !field.CanInterface() {
			continue
		}

		// Prefix handling
		fieldPrefix := prefix
		if p := fieldType.Tag.Get(prefixTag); p != emptyVal {
			fieldPrefix += p
		}

		// Nested struct
		if field.Kind() == reflect.Struct &&
			field.Type() != typeDuration &&
			field.Type() != typeTime {

			subParts := maskStruct(field, fieldPrefix, joinPath(path, fieldType.Name))
			parts = append(parts, subParts...)
			continue
		}

		envKey := fieldType.Tag.Get(envTag)
		if envKey == emptyVal {
			continue
		}

		fullKey := fieldPrefix + envKey

		var displayValue string

		if fieldType.Tag.Get(sensitiveTag) == trueVal {
			displayValue = maskedVal
		} else {
			displayValue = formatValue(field)
		}

		parts = append(parts, fmt.Sprintf("%s=%s", fullKey, displayValue))
	}

	return parts
}

// ---------------------- VALUE FORMAT ----------------------

func formatValue(v reflect.Value) string {
	// Safe handling of nil pointers to avoid interface panics
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return "<nil>"
	}

	// Interface encoding.TextMarshaler control
	if v.CanInterface() {
		var tm encoding.TextMarshaler

		// Check the value directly
		if m, ok := v.Interface().(encoding.TextMarshaler); ok {
			tm = m
		} else if v.CanAddr() {
			// Or the pointer to the value
			if m, ok := v.Addr().Interface().(encoding.TextMarshaler); ok {
				tm = m
			}
		}

		if tm != nil {
			if text, err := tm.MarshalText(); err == nil {
				return string(text)
			}
			// If MarshalText fails, we proceed with the standard fallbacks
		}
	}

	// Fallback to native types
	switch v.Kind() {
	case reflect.Pointer:
		return formatValue(v.Elem())

	case reflect.Slice:
		if v.IsNil() {
			return "[]"
		}
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = formatValue(v.Index(i))
		}
		return "[" + strings.Join(values, comma) + "]"

	case reflect.Map:
		if v.IsNil() {
			return "map[]"
		}
		var values []string
		iter := v.MapRange()
		for iter.Next() {
			k := formatValue(iter.Key())
			val := formatValue(iter.Value())
			// Hardcoding ":" here for representation purposes
			values = append(values, fmt.Sprintf("%s:%s", k, val))
		}
		return "map[" + strings.Join(values, commaAndSpace) + "]"

	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}
