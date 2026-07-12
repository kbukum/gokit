package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// APIError represents a structured error response from an LLM API.
type APIError struct {
	// Provider is the provider name (e.g., "openai", "anthropic", "gemini").
	Provider string `json:"provider"`
	// StatusCode is the HTTP status code returned.
	StatusCode int `json:"status_code"`
	// Type classifies the error (e.g., "invalid_request_error", "rate_limit_error").
	Type string `json:"type,omitempty"`
	// Message is the human-readable error description.
	Message string `json:"message"`
	// RetryAfter indicates when to retry, if the API specified it.
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("%s: [%d] %s: %s", e.Provider, e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("%s: [%d] %s", e.Provider, e.StatusCode, e.Message)
}

// IsRetryable returns true if the error is transient and the request should be retried.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusServiceUnavailable ||
		e.StatusCode == http.StatusBadGateway ||
		e.StatusCode == http.StatusGatewayTimeout ||
		e.StatusCode >= 500
}

// IsRateLimit returns true if the error is specifically a rate limit (429).
func (e *APIError) IsRateLimit() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

// IsAuth returns true if the error is an authentication/authorization failure.
func (e *APIError) IsAuth() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

// ParseOpenAIError parses an OpenAI-style error response body.
// Works for OpenAI, Azure OpenAI, vLLM, Ollama, and compatible APIs.
//
// Expected format: {"error": {"message": "...", "type": "...", "code": "..."}}
func ParseOpenAIError(provider string, statusCode int, body []byte) *APIError {
	var raw struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	apiErr := &APIError{Provider: provider, StatusCode: statusCode}

	if err := json.Unmarshal(body, &raw); err == nil && raw.Error.Message != "" {
		apiErr.Message = raw.Error.Message
		apiErr.Type = raw.Error.Type
	} else {
		apiErr.Message = string(body)
	}

	return apiErr
}

// ParseAnthropicError parses an Anthropic-style error response body.
//
// Expected format: {"type": "error", "error": {"type": "...", "message": "..."}}
func ParseAnthropicError(statusCode int, body []byte) *APIError {
	var raw struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	apiErr := &APIError{Provider: "anthropic", StatusCode: statusCode}

	if err := json.Unmarshal(body, &raw); err == nil && raw.Error.Message != "" {
		apiErr.Message = raw.Error.Message
		apiErr.Type = raw.Error.Type
	} else {
		apiErr.Message = string(body)
	}

	return apiErr
}

// ParseGeminiError parses a Google Gemini-style error response body.
//
// Expected format: {"error": {"code": 400, "message": "...", "status": "INVALID_ARGUMENT"}}
func ParseGeminiError(statusCode int, body []byte) *APIError {
	var raw struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}

	apiErr := &APIError{Provider: "gemini", StatusCode: statusCode}

	if err := json.Unmarshal(body, &raw); err == nil && raw.Error.Message != "" {
		apiErr.Message = raw.Error.Message
		apiErr.Type = raw.Error.Status
	} else {
		apiErr.Message = string(body)
	}

	return apiErr
}
