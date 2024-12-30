package notify

import "fmt"

// Error types
var (
	ErrConfigNotFound    = fmt.Errorf("notification configuration not found")
	ErrConfigInvalid     = fmt.Errorf("invalid notification configuration")
	ErrNotificationFailed = fmt.Errorf("notification failed to send")
	ErrNotEnabled        = fmt.Errorf("notification is not enabled")
)

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s - %s", e.Field, e.Message)
}

// ValidationError represents a validation error
type ValidationError struct {
	Errors []ConfigError
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("multiple validation errors (%d)", len(e.Errors))
}

// AddError adds a new validation error
func (e *ValidationError) AddError(field, message string) {
	e.Errors = append(e.Errors, ConfigError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors
func (e *ValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}
