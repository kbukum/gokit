package kafka

import (
	"errors"
	"net/http"
	"testing"
)

func TestFromKafka_Nil(t *testing.T) {
	if appErr := FromKafka(nil, "topic"); appErr != nil {
		t.Error("expected nil for nil error")
	}
}

func TestFromKafka_ConnectionError(t *testing.T) {
	err := errors.New("connection refused")
	appErr := FromKafka(err, "events")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d, want 503", appErr.HTTPStatus)
	}
	if !appErr.Retryable {
		t.Error("expected Retryable=true")
	}
	if appErr.Details["topic"] != "events" {
		t.Errorf("Details[topic] = %v, want events", appErr.Details["topic"])
	}
}

func TestFromKafka_NonRetryableError(t *testing.T) {
	err := errors.New("message too large")
	appErr := FromKafka(err, "big-topic")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %d, want 400", appErr.HTTPStatus)
	}
	if appErr.Retryable {
		t.Error("expected Retryable=false")
	}
}

func TestFromKafka_RetryableError(t *testing.T) {
	err := errors.New("request timed out")
	appErr := FromKafka(err, "timeout-topic")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d, want 503", appErr.HTTPStatus)
	}
	if !appErr.Retryable {
		t.Error("expected Retryable=true")
	}
}

func TestFromKafka_UnknownError(t *testing.T) {
	err := errors.New("some unexpected error")
	appErr := FromKafka(err, "topic")
	if appErr == nil {
		t.Fatal("expected non-nil AppError")
	}
	if appErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d, want 500", appErr.HTTPStatus)
	}
}
