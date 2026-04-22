package envconfig

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	validateTag = "validate"

	ruleMin    = "min"
	ruleMax    = "max"
	ruleOneOf  = "oneof"
	ruleRegex  = "regex"
	ruleLen    = "len"
	ruleMinLen = "minlen"
	ruleMaxLen = "maxlen"
)

var regexCache sync.Map

// validateFieldValue runs all validation checks on field after it has been set.
// It evaluates, in order:
//  1. The [Validator] interface, if implemented by the field's type.
//  2. Built-in rules declared in the validate struct tag.
//
// A nil pointer field is skipped without error.
func validateFieldValue(field reflect.Value, value, path, envKey, validateSpec string) error {
	if field.Kind() == reflect.Pointer && field.IsNil() {
		return nil
	}

	// Custom Validator interface
	if field.CanAddr() {
		if v, ok := field.Addr().Interface().(Validator); ok {
			if err := v.ValidateEnv(); err != nil {
				return fmt.Errorf("%s: validation failed for %s: %w", path, envKey, err)
			}
		}
	}
	// Also check by value (for non-pointer receivers)
	if field.CanInterface() {
		if v, ok := field.Interface().(Validator); ok {
			if err := v.ValidateEnv(); err != nil {
				return fmt.Errorf("%s: validation failed for %s: %w", path, envKey, err)
			}
		}
	}

	// Built-in tag-based rules
	if validateSpec == emptyVal {
		return nil
	}

	return applyBuiltinRules(field, value, path, envKey, validateSpec)
}

// applyBuiltinRules parses and applies the rules declared in the validate struct tag.
//
// Rules are comma-separated key=value pairs. The following rules are supported:
//
//	min=N        numeric lower bound (inclusive); applies to int, uint, and float fields
//	max=N        numeric upper bound (inclusive); applies to int, uint, and float fields
//	oneof=a|b|c  the raw string value must match one of the pipe-separated options
//	regex=RE     the raw string value must match the regular expression RE
//	len=N        exact length; applies to string, slice, and map fields
//	minlen=N     minimum length; applies to string, slice, and map fields
//	maxlen=N     maximum length; applies to string, slice, and map fields
//
// Compiled regular expressions are cached for the lifetime of the process.
func applyBuiltinRules(field reflect.Value, rawValue, path, envKey, spec string) error {
	rules, err := parseValidateSpec(spec)
	if err != nil {
		return fmt.Errorf("%s: malformed validate tag for %s: %w", path, envKey, err)
	}

	for rule, param := range rules {
		switch rule {
		case ruleMin:
			if err := checkNumericBound(field, param, path, envKey, ruleMin); err != nil {
				return err
			}
		case ruleMax:
			if err := checkNumericBound(field, param, path, envKey, ruleMax); err != nil {
				return err
			}
		case ruleOneOf:
			if err := checkOneOf(rawValue, param, path, envKey); err != nil {
				return err
			}
		case ruleRegex:
			if err := checkRegex(rawValue, param, path, envKey); err != nil {
				return err
			}
		case ruleLen:
			if err := checkLength(field, param, path, envKey, ruleLen); err != nil {
				return err
			}
		case ruleMinLen:
			if err := checkLength(field, param, path, envKey, ruleMinLen); err != nil {
				return err
			}
		case ruleMaxLen:
			if err := checkLength(field, param, path, envKey, ruleMaxLen); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unknown validation rule %q for %s", path, rule, envKey)
		}
	}

	return nil
}

// parseValidateSpec parses a validate tag value into a rule-to-parameter map.
//
// Splitting is done on commas that are immediately followed by a known rule
// prefix, so that commas inside a regex pattern (e.g. [a,b]+) are preserved.
func parseValidateSpec(spec string) (map[string]string, error) {
	rules := make(map[string]string)

	// Split on commas that are immediately followed by a known rule name and "="
	// so that regex patterns containing commas are preserved intact.
	knownPrefixes := []string{ruleMin + "=", ruleMax + "=", ruleOneOf + "=", ruleRegex + "=", ruleLen + "=", ruleMinLen + "=", ruleMaxLen + "="}
	parts := splitOnRuleBoundaries(spec, knownPrefixes)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == emptyVal {
			continue
		}
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			return nil, fmt.Errorf("rule %q has no value (expected key=value)", part)
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])
		rules[key] = val
	}

	return rules, nil
}

// splitOnRuleBoundaries splits s on commas that are immediately followed by one
// of the provided prefixes, leaving all other commas intact.
func splitOnRuleBoundaries(s string, prefixes []string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != ',' {
			continue
		}
		rest := s[i+1:]
		for _, pfx := range prefixes {
			if strings.HasPrefix(rest, pfx) {
				parts = append(parts, s[start:i])
				start = i + 1
				break
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// checkNumericBound enforces a min or max constraint on a numeric field.
// It supports signed integers, unsigned integers, and floating-point kinds.
func checkNumericBound(field reflect.Value, param, path, envKey, rule string) error {
	kind := field.Kind()

	switch {
	case isIntKind(kind):
		bound, err := strconv.ParseInt(param, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid %s value %q for %s: %w", path, rule, param, envKey, err)
		}
		return evaluateNumericRule(field.Int(), bound, rule, path, envKey)

	case isUintKind(kind):
		bound, err := strconv.ParseUint(param, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid %s value %q for %s: %w", path, rule, param, envKey, err)
		}
		return evaluateNumericRule(field.Uint(), bound, rule, path, envKey)

	case isFloatKind(kind):
		bound, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid %s value %q for %s: %w", path, rule, param, envKey, err)
		}
		return evaluateNumericRule(field.Float(), bound, rule, path, envKey)

	default:
		return fmt.Errorf("%s: min/max rules are not applicable to type %s for %s", path, field.Type(), envKey)
	}
}

// evaluateNumericRule applies a min or max bound check to a numeric value.
// T is constrained to the three numeric kinds used internally: int64, uint64,
// and float64.
func evaluateNumericRule[T int64 | uint64 | float64](v, bound T, rule, path, envKey string) error {
	if rule == ruleMin && v < bound {
		return fmt.Errorf("%s: value %v is less than min=%v for %s", path, v, bound, envKey)
	}
	if rule == ruleMax && v > bound {
		return fmt.Errorf("%s: value %v exceeds max=%v for %s", path, v, bound, envKey)
	}
	return nil
}

// checkOneOf returns an error if rawValue is not present in the pipe-separated
// list of allowed options given by param.
func checkOneOf(rawValue string, param string, path string, envKey string) error {
	options := strings.Split(param, "|")
	for _, opt := range options {
		if rawValue == opt {
			return nil
		}
	}
	return fmt.Errorf("%s: value %q is not one of [%s] for %s", path, rawValue, strings.Join(options, ", "), envKey)
}

// checkRegex returns an error if rawValue does not match the compiled form of
// pattern. Compiled expressions are stored in regexCache to avoid recompilation
// on repeated calls with the same pattern.
func checkRegex(rawValue string, pattern string, path string, envKey string) error {
	if cached, ok := regexCache.Load(pattern); ok {
		re := cached.(*regexp.Regexp)
		if !re.MatchString(rawValue) {
			return fmt.Errorf("%s: value %q does not match regex %q for %s", path, rawValue, pattern, envKey)
		}
		return nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%s: invalid regex pattern %q for %s: %w", path, pattern, envKey, err)
	}

	regexCache.Store(pattern, re)

	if !re.MatchString(rawValue) {
		return fmt.Errorf("%s: value %q does not match regex %q for %s", path, rawValue, pattern, envKey)
	}

	return nil
}

// checkLength enforces len, minlen, and maxlen constraints on string, slice, and map fields.
// For strings the byte length is used; for slices and maps the element count is used.
func checkLength(field reflect.Value, param, path, envKey, rule string) error {
	bound, err := strconv.Atoi(param)
	if err != nil {
		return fmt.Errorf("%s: invalid %s value %q for %s: %w", path, rule, param, envKey, err)
	}

	var length int
	switch field.Kind() {
	case reflect.String:
		length = len(field.String())
	case reflect.Slice:
		length = field.Len()
	case reflect.Map:
		length = len(field.MapKeys())
	default:
		return fmt.Errorf(
			"%s: len/minlen/maxlen rules are only applicable to string, slice and map fields for %s",
			path,
			envKey,
		)
	}

	switch rule {
	case ruleLen:
		if length != bound {
			return fmt.Errorf("%s: length %d does not equal len=%d for %s", path, length, bound, envKey)
		}
	case ruleMinLen:
		if length < bound {
			return fmt.Errorf("%s: length %d is less than minlen=%d for %s", path, length, bound, envKey)
		}
	case ruleMaxLen:
		if length > bound {
			return fmt.Errorf("%s: length %d exceeds maxlen=%d for %s", path, length, bound, envKey)
		}
	}

	return nil
}

// ---------------------- KIND HELPERS ----------------------

// isIntKind reports whether k is a signed integer kind.
func isIntKind(k reflect.Kind) bool {
	return k == reflect.Int || k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64
}

// isUintKind reports whether k is an unsigned integer kind.
func isUintKind(k reflect.Kind) bool {
	return k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64
}

// isFloatKind reports whether k is a floating-point kind.
func isFloatKind(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}
