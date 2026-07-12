package common

import "testing"

func TestParseOpenAIError(t *testing.T) {
	body := `{"error":{"message":"Invalid API key","type":"invalid_request_error","code":"invalid_api_key"}}`
	err := ParseOpenAIError("openai", 401, []byte(body))

	if err.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", err.Provider)
	}
	if err.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", err.StatusCode)
	}
	if err.Message != "Invalid API key" {
		t.Errorf("unexpected message: %s", err.Message)
	}
	if err.Type != "invalid_request_error" {
		t.Errorf("unexpected type: %s", err.Type)
	}
	if !err.IsAuth() {
		t.Error("expected IsAuth=true for 401")
	}
	if err.IsRetryable() {
		t.Error("401 should not be retryable")
	}
}

func TestParseAnthropicError(t *testing.T) {
	body := `{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited"}}`
	err := ParseAnthropicError(429, []byte(body))

	if err.Message != "Rate limited" {
		t.Errorf("unexpected message: %s", err.Message)
	}
	if !err.IsRateLimit() {
		t.Error("expected IsRateLimit=true for 429")
	}
	if !err.IsRetryable() {
		t.Error("429 should be retryable")
	}
}

func TestParseGeminiError(t *testing.T) {
	body := `{"error":{"code":400,"message":"Invalid request","status":"INVALID_ARGUMENT"}}`
	err := ParseGeminiError(400, []byte(body))

	if err.Message != "Invalid request" {
		t.Errorf("unexpected message: %s", err.Message)
	}
	if err.Type != "INVALID_ARGUMENT" {
		t.Errorf("unexpected type: %s", err.Type)
	}
}

func TestAPIError_String(t *testing.T) {
	err := &APIError{Provider: "openai", StatusCode: 429, Type: "rate_limit_error", Message: "Too many requests"}
	s := err.Error()
	if s != "openai: [429] rate_limit_error: Too many requests" {
		t.Errorf("unexpected error string: %s", s)
	}

	err2 := &APIError{Provider: "gemini", StatusCode: 500, Message: "Internal error"}
	s2 := err2.Error()
	if s2 != "gemini: [500] Internal error" {
		t.Errorf("unexpected error string: %s", s2)
	}
}

func TestParseOpenAIError_InvalidJSON(t *testing.T) {
	err := ParseOpenAIError("openai", 500, []byte("not json"))
	if err.Message != "not json" {
		t.Errorf("expected raw body as message, got %q", err.Message)
	}
}
