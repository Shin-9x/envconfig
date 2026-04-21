package envconfig

import (
	"fmt"
	"reflect"
	"strings"
)

const sep = ","
const sepAndComma = sep + ", "

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

	return fmt.Sprintf("%s: {%s}", rv.Type().Name(), strings.Join(parts, sepAndComma))
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
	switch v.Kind() {

	case reflect.Pointer:
		if v.IsNil() {
			return "<nil>"
		}
		return formatValue(v.Elem())

	case reflect.Slice:
		if v.IsNil() {
			return "[]"
		}
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = formatValue(v.Index(i))
		}
		return "[" + strings.Join(values, sep) + "]"

	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}
