package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/errors"
)

// Validator collects field violations from hand-written business rules and turns
// them into a single AppError. It produces the shared errors.Violation shape, so
// a manual rule and a protovalidate rule land on form fields the same way.
type Validator struct {
	violations []errors.Violation
	cause      error
}

// New creates a new Validator.
func New() *Validator {
	return &Validator{violations: make([]errors.Violation, 0)}
}

// AddViolation records a field problem with a semantic reason.
func (v *Validator) AddViolation(field string, reason errors.ViolationReason, message string) {
	v.violations = append(v.violations, errors.Violation{Field: field, Reason: reason, Message: message})
}

// AddError records a field problem without a machine rule id.
func (v *Validator) AddError(field, message string) {
	v.AddViolation(field, errors.ViolationInvalidValue, message)
}

// HasErrors returns true if there are violations.
func (v *Validator) HasErrors() bool {
	return v.cause != nil || len(v.violations) > 0
}

// Violations returns all collected violations.
func (v *Validator) Violations() []errors.Violation {
	return v.violations
}

// Validate returns an AppError carrying the collected violations, or nil when
// there are none.
func (v *Validator) Validate() *errors.AppError {
	if v.cause != nil {
		return errors.Internal(v.cause)
	}
	if !v.HasErrors() {
		return nil
	}

	messages := make([]string, len(v.violations))
	for i, viol := range v.violations {
		messages[i] = fmt.Sprintf("%s: %s", viol.Field, viol.Message)
	}

	return errors.Validation(strings.Join(messages, "; ")).WithViolations(v.violations...)
}

// Required checks if a string is non-empty.
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddViolation(field, ReasonForRule("required"), "is required")
	}
	return v
}

// RequiredUUID checks if a string is a valid non-nil UUID.
func (v *Validator) RequiredUUID(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.AddViolation(field, ReasonForRule("required"), "is required")
		return v
	}

	parsed, err := uuid.Parse(value)
	if err != nil {
		v.AddViolation(field, ReasonForRule("uuid"), "must be a valid UUID")
		return v
	}

	if parsed == uuid.Nil {
		v.AddViolation(field, ReasonForRule("uuid"), "must not be empty")
	}

	return v
}

// OptionalUUID checks if a non-empty string is a valid UUID.
func (v *Validator) OptionalUUID(field, value string) *Validator {
	if value == "" {
		return v
	}
	if _, err := uuid.Parse(value); err != nil {
		v.AddViolation(field, ReasonForRule("uuid"), "must be a valid UUID")
	}
	return v
}

// MaxLength checks if a string is within max length.
func (v *Validator) MaxLength(field, value string, maxLen int) *Validator {
	if len(value) > maxLen {
		v.AddViolation(field, ReasonForRule("max_len"), fmt.Sprintf("must be %d characters or less", maxLen))
	}
	return v
}

// MinLength checks if a string meets minimum length.
func (v *Validator) MinLength(field, value string, minLen int) *Validator {
	if len(value) < minLen {
		v.AddViolation(field, ReasonForRule("min_len"), fmt.Sprintf("must be at least %d characters", minLen))
	}
	return v
}

// Email checks if a string is a valid email address.
func (v *Validator) Email(field, value string) *Validator {
	local, domain, ok := strings.Cut(value, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") ||
		!strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		v.AddViolation(field, ReasonForRule("email"), "must be a valid email address")
	}
	return v
}

// URL checks if a string is a valid absolute URL.
func (v *Validator) URL(field, value string) *Validator {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		v.AddViolation(field, ReasonForRule("url"), "must be a valid URL")
	}
	return v
}

// Before checks if a time is before a deadline.
func (v *Validator) Before(field string, value, deadline time.Time) *Validator {
	if !value.Before(deadline) {
		v.AddViolation(field, ReasonForRule("before"), fmt.Sprintf("must be before %s", deadline.Format(time.RFC3339Nano)))
	}
	return v
}

// After checks if a time is after a floor.
func (v *Validator) After(field string, value, floor time.Time) *Validator {
	if !value.After(floor) {
		v.AddViolation(field, ReasonForRule("after"), fmt.Sprintf("must be after %s", floor.Format(time.RFC3339Nano)))
	}
	return v
}

// Range checks if a number is within a range.
func (v *Validator) Range(field string, value, minVal, maxVal int) *Validator {
	if value < minVal || value > maxVal {
		v.AddViolation(field, ReasonForRule("range"), fmt.Sprintf("must be between %d and %d", minVal, maxVal))
	}
	return v
}

// Min checks if a number meets minimum value.
func (v *Validator) Min(field string, value, minVal int) *Validator {
	if value < minVal {
		v.AddViolation(field, ReasonForRule("min"), fmt.Sprintf("must be at least %d", minVal))
	}
	return v
}

// Max checks if a number is within max value.
func (v *Validator) Max(field string, value, maxVal int) *Validator {
	if value > maxVal {
		v.AddViolation(field, ReasonForRule("max"), fmt.Sprintf("must be %d or less", maxVal))
	}
	return v
}

// Pattern checks if a string matches a regex pattern.
func (v *Validator) Pattern(field, value, pattern string) *Validator {
	if value == "" {
		return v
	}
	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		v.cause = err
		return v
	}
	if !matched {
		v.AddViolation(field, ReasonForRule("pattern"), "does not match required format")
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
	v.AddViolation(field, ReasonForRule("one_of"), fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
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
		return uuid.Nil, errors.MissingField(field)
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, errors.InvalidFormat(field, "UUID")
	}

	return id, nil
}
