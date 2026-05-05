package middleware

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/resilience"
)

func TestRetryHandler_SucceedsFirstAttempt(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
	}

	wrapped := RetryHandler(handler, cfg)
	msg := messaging.Message{Topic: "t", Headers: map[string]string{}}
	err := wrapped(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryHandler_RetriesOnError(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return errors.New("transient")
		}
		return nil
	}

	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
	}

	wrapped := RetryHandler(handler, cfg)
	msg := messaging.Message{Topic: "t", Headers: map[string]string{}}
	err := wrapped(context.Background(), msg)
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryHandler_SetsRetryCountHeader(t *testing.T) {
	var lastHeaders map[string]string
	var calls int32
	handler := func(_ context.Context, msg messaging.Message) error {
		n := atomic.AddInt32(&calls, 1)
		lastHeaders = msg.Headers
		if n < 3 {
			return errors.New("transient")
		}
		return nil
	}

	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
	}

	wrapped := RetryHandler(handler, cfg)
	msg := messaging.Message{Topic: "t", Headers: map[string]string{"existing": "value"}}
	_ = wrapped(context.Background(), msg)

	rc, ok := lastHeaders["x-retry-count"]
	if !ok {
		t.Fatal("expected x-retry-count header to be set on final attempt")
	}
	count, err := strconv.Atoi(rc)
	if err != nil {
		t.Fatalf("x-retry-count not an integer: %v", err)
	}
	if count != 2 {
		t.Errorf("expected retry count 2, got %d", count)
	}
	// Original headers must be preserved.
	if lastHeaders["existing"] != "value" {
		t.Errorf("existing header lost: %v", lastHeaders)
	}
}

func TestRetryHandler_OnExhaustedCalled(t *testing.T) {
	handler := func(_ context.Context, _ messaging.Message) error {
		return errors.New("always fails")
	}

	var exhaustedMsg messaging.Message
	var exhaustedErr error
	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    2,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
		OnExhausted: func(_ context.Context, msg messaging.Message, err error) error {
			exhaustedMsg = msg
			exhaustedErr = err
			return nil
		},
	}

	wrapped := RetryHandler(handler, cfg)
	msg := messaging.Message{Topic: "orders", Key: "k1", Headers: map[string]string{}}
	_ = wrapped(context.Background(), msg)

	if exhaustedErr == nil {
		t.Fatal("expected OnExhausted to be called")
	}
	if exhaustedMsg.Topic != "orders" {
		t.Errorf("OnExhausted msg.Topic = %q, want orders", exhaustedMsg.Topic)
	}
}

func TestRetryHandler_OnExhaustedNotCalledOnSuccess(t *testing.T) {
	handler := func(_ context.Context, _ messaging.Message) error { return nil }

	called := false
	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.DefaultRetryConfig(),
		OnExhausted: func(_ context.Context, _ messaging.Message, _ error) error {
			called = true
			return nil
		},
	}

	wrapped := RetryHandler(handler, cfg)
	_ = wrapped(context.Background(), messaging.Message{Headers: map[string]string{}})

	if called {
		t.Error("OnExhausted should not be called when handler succeeds")
	}
}

func TestRetryHandler_NilHeaders(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return errors.New("fail")
		}
		return nil
	}

	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
	}

	wrapped := RetryHandler(handler, cfg)
	// Pass message with nil headers — should not panic.
	err := wrapped(context.Background(), messaging.Message{Topic: "t"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRetryHandler_DoesNotMutateOriginalHeaders(t *testing.T) {
	handler := func(_ context.Context, _ messaging.Message) error {
		return errors.New("fail")
	}

	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{
			MaxAttempts:    2,
			InitialBackoff: time.Millisecond,
			BackoffFactor:  2.0,
		},
	}

	original := map[string]string{"key": "val"}
	msg := messaging.Message{Topic: "t", Headers: original}

	wrapped := RetryHandler(handler, cfg)
	_ = wrapped(context.Background(), msg)

	if _, ok := original["x-retry-count"]; ok {
		t.Error("original headers map was mutated by retry middleware")
	}
}

func TestRetryHandler_OnExhaustedSuccessSwallowsTerminalError(t *testing.T) {
	handler := func(_ context.Context, _ messaging.Message) error { return errors.New("poison") }
	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{MaxAttempts: 1, InitialBackoff: time.Millisecond},
		OnExhausted: func(context.Context, messaging.Message, error) error { return nil },
	}

	if err := RetryHandler(handler, cfg)(context.Background(), messaging.Message{Headers: map[string]string{}}); err != nil {
		t.Fatalf("expected exhausted handler success to swallow terminal error, got %v", err)
	}
}

func TestRetryHandler_OnExhaustedFailurePropagates(t *testing.T) {
	dlqErr := errors.New("dlq publish failed")
	handler := func(_ context.Context, _ messaging.Message) error { return errors.New("poison") }
	cfg := RetryMiddlewareConfig{
		RetryConfig: resilience.RetryConfig{MaxAttempts: 1, InitialBackoff: time.Millisecond},
		OnExhausted: func(context.Context, messaging.Message, error) error { return dlqErr },
	}

	err := RetryHandler(handler, cfg)(context.Background(), messaging.Message{Headers: map[string]string{}})
	if !errors.Is(err, dlqErr) {
		t.Fatalf("expected DLQ error propagation, got %v", err)
	}
}
