package envconfig

// Validator is the interface implemented by types that can validate
// themselves after being parsed from an environment variable string.
//
// ValidateEnv is called after the value has been set on the field.
// Return a descriptive error if the value doesn't satisfy the constraint.
//
// Example:
//
//	type Port struct{ value int }
//
//	func (p Port) ValidateEnv() error {
//	    if p.value < 1 || p.value > 65535 {
//	        return fmt.Errorf("port must be between 1 and 65535, got %d", p.value)
//	    }
//	    return nil
//	}
type Validator interface {
	ValidateEnv() error
}
