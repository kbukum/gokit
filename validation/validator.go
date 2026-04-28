package validation

import (
	"fmt"
	"regexp"
	"strings"
	"time"

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

// InRange checks if a number is within an inclusive range.
func (v *Validator) InRange(field string, value, minVal, maxVal int) *Validator {
	if value < minVal || value > maxVal {
		v.AddError(field, fmt.Sprintf("must be between %d and %d", minVal, maxVal))
	}
	return v
}

// MinValue checks if a number meets the minimum value.
func (v *Validator) MinValue(field string, value, minVal int) *Validator {
	if value < minVal {
		v.AddError(field, fmt.Sprintf("must be at least %d", minVal))
	}
	return v
}

// MaxValue checks if a number does not exceed the maximum value.
func (v *Validator) MaxValue(field string, value, maxVal int) *Validator {
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
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(fmt.Sprintf("validation: invalid regex pattern %q: %v", pattern, err))
	}
	if !re.MatchString(value) {
		v.AddError(field, "does not match required format")
	}
	return v
}

// Email checks if a string is a valid email address.
func (v *Validator) Email(field, value string) *Validator {
	if value == "" {
		return v
	}
	// Simple email validation: local@domain.tld
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(value) {
		v.AddError(field, "must be a valid email address")
	}
	return v
}

// URL checks if a string is a valid HTTP or HTTPS URL.
func (v *Validator) URL(field, value string) *Validator {
	if value == "" {
		return v
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		v.AddError(field, "must be a valid URL")
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

// Before checks that value (RFC 3339 datetime string) is strictly before deadline.
// Empty values are skipped (use Required first).
func (v *Validator) Before(field, value, deadline string) *Validator {
	if value == "" {
		return v
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		v.AddError(field, "must be a valid datetime")
		return v
	}
	d, err := time.Parse(time.RFC3339, deadline)
	if err != nil {
		return v
	}
	if !t.Before(d) {
		v.AddError(field, fmt.Sprintf("must be before %s", deadline))
	}
	return v
}

// After checks that value (RFC 3339 datetime string) is strictly after floor.
// Empty values are skipped (use Required first).
func (v *Validator) After(field, value, floor string) *Validator {
	if value == "" {
		return v
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		v.AddError(field, "must be a valid datetime")
		return v
	}
	f, err := time.Parse(time.RFC3339, floor)
	if err != nil {
		return v
	}
	if !t.After(f) {
		v.AddError(field, fmt.Sprintf("must be after %s", floor))
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
