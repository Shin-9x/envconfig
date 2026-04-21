package envconfig

// Unmarshaler is the interface implemented by types that can parse
// themselves from a raw environment variable string.
//
// UnmarshalEnv receives the raw string value (never empty) and is responsible for populating the receiver.
// Return a descriptive error if the value is invalid.
//
// Example:
//
//	type LogLevel struct{ level string }
//
//	func (l *LogLevel) UnmarshalEnv(value string) error {
//	    switch value {
//	    case "DEBUG", "INFO", "WARN", "ERROR":
//	        l.level = value
//	        return nil
//	    default:
//	        return fmt.Errorf("invalid log level %q, must be one of DEBUG, INFO, WARN, ERROR", value)
//	    }
//	}
type Unmarshaler interface {
	UnmarshalEnv(value string) error
}
