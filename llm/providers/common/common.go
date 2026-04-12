// Package common provides shared utilities for LLM provider implementations.
//
// These utilities handle cross-cutting concerns like API error parsing,
// rate limit header extraction, and token counting heuristics that are
// common across OpenAI, Anthropic, Gemini, and other providers.
package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

// RateLimitInfo holds rate limit metadata extracted from HTTP response headers.
type RateLimitInfo struct {
	// Limit is the max requests allowed in the window.
	Limit int
	// Remaining is the number of requests left in the current window.
	Remaining int
	// ResetAt is when the rate limit window resets.
	ResetAt time.Time
	// RetryAfter is how long to wait before retrying (for 429 responses).
	RetryAfter time.Duration
}

// ParseRateLimitHeaders extracts rate limit info from response headers.
// Supports OpenAI-style headers (x-ratelimit-*) and standard Retry-After.
func ParseRateLimitHeaders(headers http.Header) *RateLimitInfo {
	info := &RateLimitInfo{}
	found := false

	if v := headers.Get("x-ratelimit-limit-requests"); v != "" {
		info.Limit, _ = strconv.Atoi(v)
		found = true
	}
	if v := headers.Get("x-ratelimit-remaining-requests"); v != "" {
		info.Remaining, _ = strconv.Atoi(v)
		found = true
	}
	if v := headers.Get("x-ratelimit-reset-requests"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			info.ResetAt = time.Now().Add(d)
			found = true
		}
	}
	if v := headers.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			info.RetryAfter = time.Duration(secs) * time.Second
			found = true
		}
	}

	if !found {
		return nil
	}
	return info
}

// EstimateTokens provides a rough token count estimate using the
// ~4 characters per token heuristic. This is useful for pre-flight
// checks before sending requests. For accurate counts, use the
// provider's native tokenizer.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
