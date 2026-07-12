package errors

import (
	stderrors "errors"
	"strings"
	"sync/atomic"
	"unicode"
)

// typeBaseURI holds the configurable base URI for problem type URIs.
// Callers that need a different base must call SetTypeBaseURI before
// any error response is rendered.
const defaultTypeBaseURI = "https://gokit.dev/errors/"

var typeBaseURI atomic.Value

// SetTypeBaseURI sets the base URI used when constructing ProblemDetail.Type.
// The uri is normalised to always end with "/".
func SetTypeBaseURI(uri string) {
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	typeBaseURI.Store(uri)
}

// GetTypeBaseURI returns the current base URI, materializing the default on
// first call.
func GetTypeBaseURI() string {
	if v := typeBaseURI.Load(); v != nil {
		return v.(string)
	}
	// CAS-style lazy init: only one goroutine wins, others observe the value.
	typeBaseURI.CompareAndSwap(nil, defaultTypeBaseURI)
	return typeBaseURI.Load().(string)
}

// ProblemDetail is the RFC 9457 Problem Details response type.
type ProblemDetail struct {
	// Type is a URI identifying the problem type.
	Type string `json:"type"`
	// Title is a short human-readable summary of the problem type.
	Title string `json:"title"`
	// Status is the HTTP status code.
	Status int `json:"status"`
	// Detail is a human-readable explanation specific to this occurrence.
	Detail string `json:"detail"`
	// Instance is an optional URI identifying the specific occurrence.
	Instance string `json:"instance,omitempty"`
	// Code is the machine-readable error code.
	Code ErrorCode `json:"code"`
	// Retryable indicates whether the client may retry the request.
	Retryable bool `json:"retryable"`
	// Details carries RFC 9457 problem-detail extension members (arbitrary
	// JSON). This is a deliberate, documented opaque-value exception to the
	// no-any rule; see AppError.Details.
	Details map[string]any `json:"details,omitempty"`
}

// ToProblemDetail converts an AppError to a ProblemDetail following RFC 9457.
// Instance is left empty and should be populated by the HTTP middleware.
func (e *AppError) ToProblemDetail() ProblemDetail {
	kebab := strings.ToLower(strings.ReplaceAll(string(e.Code), "_", "-"))
	return ProblemDetail{
		Type:      GetTypeBaseURI() + kebab,
		Title:     codeToTitle(e.Code),
		Status:    e.HTTPStatus,
		Detail:    e.Message,
		Code:      e.Code,
		Retryable: e.Retryable,
		Details:   e.Details,
	}
}

// codeToTitle converts a SCREAMING_SNAKE_CASE error code to a title-cased string.
// e.g. NOT_FOUND → "Not Found", INTERNAL_ERROR → "Internal Error".
func codeToTitle(code ErrorCode) string {
	parts := strings.Split(string(code), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(strings.ToLower(p))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

// IsAppError checks if an error is an AppError.
func IsAppError(err error) bool {
	var appErr *AppError
	return stderrors.As(err, &appErr)
}

// AsAppError converts an error to an AppError if possible.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
