package render

import (
	"encoding/json"
	"fmt"

	yaml "go.yaml.in/yaml/v3"

	"github.com/kbukum/gokit/errors"
)

// ExitCode is the process exit-code convention shared by gokit CLIs.
type ExitCode int

const (
	// ExitSuccess is a successful command.
	ExitSuccess ExitCode = 0
	// ExitFailure is an unclassified failure.
	ExitFailure ExitCode = 1
	// ExitUsage is invalid command input or configuration.
	ExitUsage ExitCode = 2
	// ExitPermission is an authentication or authorization failure.
	ExitPermission ExitCode = 3
	// ExitNotFound is a requested resource that was not found.
	ExitNotFound ExitCode = 4
	// ExitConflict is a conflict with the current state.
	ExitConflict ExitCode = 5
	// ExitUnavailable is a remote dependency or service failure.
	ExitUnavailable ExitCode = 69
	// ExitRateLimited is a rate-limited command.
	ExitRateLimited ExitCode = 75
	// ExitTimeout is a command that timed out.
	ExitTimeout ExitCode = 124
	// ExitCanceled is a command that was canceled.
	ExitCanceled ExitCode = 130
)

// Int returns the exit code as an integer suitable for os.Exit.
func (c ExitCode) Int() int { return int(c) }

// ExitCodeForError maps an error's code onto the CLI exit-code convention.
//
// It resolves the underlying [github.com/kbukum/gokit/errors.AppError] via errors.As;
// a nil error maps to [ExitSuccess] and any non-AppError to [ExitFailure].
func ExitCodeForError(err error) ExitCode {
	if err == nil {
		return ExitSuccess
	}
	appErr, ok := errors.AsAppError(err)
	if !ok {
		return ExitFailure
	}
	return exitCodeForCode(appErr.Code)
}

func exitCodeForCode(code errors.ErrorCode) ExitCode {
	switch code {
	case errors.ErrCodeInvalidInput, errors.ErrCodeInvalidFormat, errors.ErrCodeMissingField:
		return ExitUsage
	case errors.ErrCodeUnauthorized, errors.ErrCodeForbidden,
		errors.ErrCodeTokenExpired, errors.ErrCodeInvalidToken:
		return ExitPermission
	case errors.ErrCodeNotFound:
		return ExitNotFound
	case errors.ErrCodeConflict, errors.ErrCodeAlreadyExists:
		return ExitConflict
	case errors.ErrCodeServiceUnavailable, errors.ErrCodeConnectionFailed,
		errors.ErrCodeExternalService:
		return ExitUnavailable
	case errors.ErrCodeRateLimited:
		return ExitRateLimited
	case errors.ErrCodeTimeout:
		return ExitTimeout
	case errors.ErrCodeCanceled:
		return ExitCanceled
	default:
		return ExitFailure
	}
}

// ErrorRenderer renders [github.com/kbukum/gokit/errors.AppError] values consistently for command-line applications.
type ErrorRenderer struct {
	format OutputFormat
}

// NewErrorRenderer creates a renderer for the requested output format.
func NewErrorRenderer(format OutputFormat) ErrorRenderer {
	return ErrorRenderer{format: format}
}

// errorEnvelope is the machine-readable projection of an error, shared by the JSON
// and YAML formats.
type errorEnvelope struct {
	Code       errors.ErrorCode `json:"code" yaml:"code"`
	Message    string           `json:"message" yaml:"message"`
	Retryable  bool             `json:"retryable" yaml:"retryable"`
	HTTPStatus int              `json:"http_status" yaml:"http_status"`
	ExitCode   int              `json:"exit_code" yaml:"exit_code"`
	Details    map[string]any   `json:"details,omitempty" yaml:"details,omitempty"`
}

// Render renders an error and returns the matching CLI exit code.
//
// The same exit code is returned regardless of format, so callers can render in any format
// and still exit consistently. A non-AppError is first wrapped as an internal AppError
// so every error renders through the same envelope.
func (r ErrorRenderer) Render(err error) (string, ExitCode) {
	if err == nil {
		return "", ExitSuccess
	}
	appErr := errors.Wrap(err)
	exit := exitCodeForCode(appErr.Code)
	switch r.format {
	case FormatJSON:
		envelope := newEnvelope(appErr, exit)
		encoded, marshalErr := json.Marshal(envelope)
		if marshalErr != nil {
			return fallbackJSON(appErr, exit), exit
		}
		return string(encoded), exit
	case FormatYAML:
		envelope := newEnvelope(appErr, exit)
		encoded, marshalErr := yaml.Marshal(envelope)
		if marshalErr != nil {
			return fallbackYAML(appErr, exit), exit
		}
		return string(encoded), exit
	default:
		return fmt.Sprintf("error[%s]: %s", appErr.Code, appErr.Message), exit
	}
}

func newEnvelope(err *errors.AppError, exit ExitCode) errorEnvelope {
	return errorEnvelope{
		Code:       err.Code,
		Message:    err.Message,
		Retryable:  err.Retryable,
		HTTPStatus: err.HTTPStatus,
		ExitCode:   exit.Int(),
		Details:    err.Details,
	}
}

// fallbackJSON renders a Details-free envelope when the full envelope fails to marshal (an unmarshalable detail value).
// Dropping Details leaves only JSON-safe scalars; the final string is a defensive last resort.
func fallbackJSON(err *errors.AppError, exit ExitCode) string {
	envelope := newEnvelope(err, exit)
	envelope.Details = nil
	if encoded, marshalErr := json.Marshal(envelope); marshalErr == nil {
		return string(encoded)
	}
	return fmt.Sprintf(`{"code":%q,"exit_code":%d}`, err.Code, exit.Int())
}

// fallbackYAML mirrors [fallbackJSON] for the YAML format.
func fallbackYAML(err *errors.AppError, exit ExitCode) string {
	envelope := newEnvelope(err, exit)
	envelope.Details = nil
	if encoded, marshalErr := yaml.Marshal(envelope); marshalErr == nil {
		return string(encoded)
	}
	return fmt.Sprintf("code: %s\nexit_code: %d\n", err.Code, exit.Int())
}
