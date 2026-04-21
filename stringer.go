package envconfig

import (
	"fmt"
	"reflect"
	"strings"
)

const sep = ", "

// Mask returns a string representation of any struct,
// masking fields tagged as sensitive:"true".
func Mask(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	rt := rv.Type()

	parts := make([]string, 0, rv.NumField())

	for i := range rv.NumField() {
		fieldType := rt.Field(i)

		envKey := fieldType.Tag.Get(envTag)
		if envKey == emptyVal {
			continue
		}

		var displayValue string
		if fieldType.Tag.Get(sensitiveTag) == trueVal {
			displayValue = maskedVal
		} else {
			displayValue = fmt.Sprintf("%v", rv.Field(i).Interface())
		}

		parts = append(parts, fmt.Sprintf("%s=%s", envKey, displayValue))
	}

	return fmt.Sprintf("%s: {%s}", rt.Name(), strings.Join(parts, sep))
}
