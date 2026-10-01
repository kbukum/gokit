package validation

import (
	stderrors "errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/kbukum/gokit/errors"
)

// StructValidator validates structs by their `validate:"..."` tags and reports
// failures as the shared errors.Violation shape. It owns its own underlying
// validator instance, so callers inject it at the composition root instead of
// reaching for a package-global singleton.
type StructValidator struct {
	validate *validator.Validate
}

// NewStructValidator builds a StructValidator that reports field names from json
// tags, matching how the fields travel on the wire.
func NewStructValidator() *StructValidator {
	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
	return &StructValidator{validate: validate}
}

// Validate validates a struct using its tags and returns an AppError carrying the
// violations, or nil when the struct is valid.
func (sv *StructValidator) Validate(s any) *errors.AppError {
	err := sv.validate.Struct(s)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !stderrors.As(err, &validationErrors) {
		return errors.Internal(err)
	}

	violations := make([]errors.Violation, 0, len(validationErrors))
	messages := make([]string, 0, len(validationErrors))
	for _, e := range validationErrors {
		field := violationField(e)
		message := formatValidationError(e)
		violations = append(violations, errors.Violation{Field: field, Reason: ReasonForRule(e.Tag()), Message: message})
		messages = append(messages, field+": "+message)
	}

	return errors.Validation(strings.Join(messages, "; ")).WithViolations(violations...)
}

// violationField returns the canonical, json-tag-aware path to the failing field
// so nested and indexed fields stay unambiguous (for example "billing.email" or
// "samples[2].prompt"), dropping only the leading root struct name.
func violationField(e validator.FieldError) string {
	ns := e.Namespace()
	if idx := strings.IndexByte(ns, '.'); idx >= 0 {
		return ns[idx+1:]
	}
	return e.Field()
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
