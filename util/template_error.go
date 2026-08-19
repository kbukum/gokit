package util

import "fmt"

// TemplateErrorKind classifies a TemplateError.
type TemplateErrorKind int

const (
	// TemplateErrorUnclosedPlaceholder marks a placeholder missing its closing brace.
	TemplateErrorUnclosedPlaceholder TemplateErrorKind = iota
	// TemplateErrorUnmatchedClosingBrace marks a stray closing brace with no opener.
	TemplateErrorUnmatchedClosingBrace
	// TemplateErrorEmptyPlaceholder marks a placeholder with an empty name.
	TemplateErrorEmptyPlaceholder
	// TemplateErrorUnknownPlaceholder marks a placeholder not in the typed set.
	TemplateErrorUnknownPlaceholder
	// TemplateErrorRender marks a failure returned by a render callback.
	TemplateErrorRender
	// TemplateErrorMissingVariable marks a dynamic variable with no value.
	TemplateErrorMissingVariable
)

// TemplateError describes a template parse or render failure. It is comparable, so
// callers can match a specific failure with ==.
type TemplateError struct {
	// Kind is the failure category.
	Kind TemplateErrorKind
	// Detail is the offending template, placeholder, or variable name.
	Detail string
}

// Error implements error with a message matching the failure kind.
func (e TemplateError) Error() string {
	switch e.Kind {
	case TemplateErrorUnclosedPlaceholder:
		return fmt.Sprintf("unclosed placeholder in '%s'", e.Detail)
	case TemplateErrorUnmatchedClosingBrace:
		return fmt.Sprintf("unmatched closing placeholder brace in '%s'", e.Detail)
	case TemplateErrorEmptyPlaceholder:
		return "placeholder cannot be empty"
	case TemplateErrorUnknownPlaceholder:
		return fmt.Sprintf("unknown placeholder '%s'", e.Detail)
	case TemplateErrorRender:
		return fmt.Sprintf("template render failed: %s", e.Detail)
	case TemplateErrorMissingVariable:
		return fmt.Sprintf("missing template variable %q", e.Detail)
	default:
		return "template error"
	}
}
