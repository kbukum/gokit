package validation

import (
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/kbukum/gokit/errors"
)

var (
	validate *validator.Validate
	once     sync.Once
)

// getValidator returns the singleton validator instance.
func getValidator() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())

		// Use json tag names for field names in error messages
		validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" || name == "" {
				return toSnakeCase(fld.Name)
			}
			return name
		})
	})
	return validate
}

// Validate validates a struct using struct tags.
// Uses tags like `validate:"required,email,max=255"`.
func Validate(s any) error {
	v := getValidator()
	err := v.Struct(s)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return errors.Validation("validation failed")
	}

	// Build detailed error message
	fieldErrors := make([]FieldError, 0, len(validationErrors))
	messages := make([]string, 0, len(validationErrors))

	for _, e := range validationErrors {
		fieldName := toSnakeCase(e.Field())
		message := formatValidationError(e)
		fieldErrors = append(fieldErrors, FieldError{
			Field:   fieldName,
			Message: message,
		})
		messages = append(messages, fieldName+": "+message)
	}

	appErr := errors.Validation(strings.Join(messages, "; "))
	appErr.Details = map[string]any{
		"fields": fieldErrors,
	}

	return appErr
}

// formatValidationError creates a human-readable error message.
func formatValidationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + e.Param() + " characters"
	case "max":
		return "must be at most " + e.Param() + " characters"
	case "url":
		return "must be a valid URL"
	case "uuid":
		return "must be a valid UUID"
	case "oneof":
		return "must be one of: " + e.Param()
	default:
		return "is invalid"
	}
}

// toSnakeCase converts a field name to snake_case.
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		if r >= 'A' && r <= 'Z' {
			result.WriteRune(r + 32) // lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
