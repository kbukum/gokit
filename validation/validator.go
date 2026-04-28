package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/errors"
)

// Validator collects validation errors.
type Validator struct {
	errors []FieldError
}

// FieldError represents a validation error for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// New creates a new Validator.
func New() *Validator {
	return &Validator{
		errors: make([]FieldError, 0),
	}
}

// AddError adds a field error.
func (v *Validator) AddError(field, message string) {
	v.errors = append(v.errors, FieldError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors.
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors.
func (v *Validator) Errors() []FieldError {
	return v.errors
}

// Validate returns an AppError if there are validation errors, nil otherwise.
func (v *Validator) Validate() *errors.AppError {
	if !v.HasErrors() {
		return nil
	}

	// Build error message from all field errors
	messages := make([]string, len(v.errors))
	for i, e := range v.errors {
		messages[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}

	appErr := errors.Validation(strings.Join(messages, "; "))
	appErr.Details = map[string]any{
		"fields": v.errors,
	}

	return appErr
}

// Required checks if a string is non-empty.
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddError(field, "is required")
	}
	return v
}

// RequiredUUID checks if a string is a valid non-nil UUID.
func (v *Validator) RequiredUUID(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddError(field, "is required")
		return v
	}

	parsed, err := uuid.Parse(value)
	if err != nil {
		v.AddError(field, "must be a valid UUID")
		return v
	}

	if parsed == uuid.Nil {
		v.AddError(field, "must not be empty")
	}

	return v
}

// OptionalUUID checks if a non-empty string is a valid UUID.
func (v *Validator) OptionalUUID(field, value string) *Validator {
	if value == "" {
		return v
	}
	if _, err := uuid.Parse(value); err != nil {
		v.AddError(field, "must be a valid UUID")
	}
	return v
}

// MaxLength checks if a string is within max length.
func (v *Validator) MaxLength(field, value string, maxLen int) *Validator {
	if len(value) > maxLen {
		v.AddError(field, fmt.Sprintf("must be %d characters or less", maxLen))
	}
	return v
}

// MinLength checks if a string meets minimum length.
func (v *Validator) MinLength(field, value string, minLen int) *Validator {
	if len(value) < minLen {
		v.AddError(field, fmt.Sprintf("must be at least %d characters", minLen))
	}
	return v
}

// Range checks if a number is within a range.
func (v *Validator) Range(field string, value, minVal, maxVal int) *Validator {
	if value < minVal || value > maxVal {
		v.AddError(field, fmt.Sprintf("must be between %d and %d", minVal, maxVal))
	}
	return v
}

// Min checks if a number meets minimum value.
func (v *Validator) Min(field string, value, minVal int) *Validator {
	if value < minVal {
		v.AddError(field, fmt.Sprintf("must be at least %d", minVal))
	}
	return v
}

// Max checks if a number is within max value.
func (v *Validator) Max(field string, value, maxVal int) *Validator {
	if value > maxVal {
		v.AddError(field, fmt.Sprintf("must be %d or less", maxVal))
	}
	return v
}

// Pattern checks if a string matches a regex pattern.
func (v *Validator) Pattern(field, value, pattern string) *Validator {
	if value == "" {
		return v
	}
	matched, err := regexp.MatchString(pattern, value)
	if err != nil || !matched {
		v.AddError(field, "does not match required format")
	}
	return v
}

// OneOf checks if a value is one of the allowed values.
func (v *Validator) OneOf(field, value string, allowed []string) *Validator {
	if value == "" {
		return v
	}
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	v.AddError(field, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
	return v
}

// Custom applies a custom validation condition.
func (v *Validator) Custom(condition bool, field, message string) *Validator {
	if !condition {
		v.AddError(field, message)
	}
	return v
}

// Required validates a single required field and returns an error if empty.
func Required(field, value string) error {
	v := New().Required(field, value)
	if appErr := v.Validate(); appErr != nil {
		return appErr
	}
	return nil
}

// ValidateUUID validates and parses a UUID string.
func ValidateUUID(field, value string) (uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return uuid.Nil, errors.Validation(fmt.Sprintf("%s is required", field))
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, errors.Validation(fmt.Sprintf("%s must be a valid UUID", field))
	}

	return id, nil
}
