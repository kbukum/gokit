package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("unmarshal log line %q: %v", buf.String(), err)
	}
	return m
}

func TestContextWithInjectorsFoldIntoFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = ContextWithTraceID(ctx, "trace-1")
	ctx = ContextWithSpanID(ctx, "span-1")
	ctx = ContextWithRequestID(ctx, "req-1")
	ctx = ContextWithUserID(ctx, "user-1")
	ctx = ContextWithCorrelationID(ctx, "corr-1")

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.WithContext(ctx).Info("hello")

	m := decodeLogLine(t, &buf)
	for k, want := range map[string]string{
		FieldTraceID:       "trace-1",
		FieldSpanID:        "span-1",
		FieldRequestID:     "req-1",
		FieldUserID:        "user-1",
		FieldCorrelationID: "corr-1",
	} {
		if got, _ := m[k].(string); got != want {
			t.Errorf("field %q = %q, want %q", k, got, want)
		}
	}
}

func TestComponentSpan(t *testing.T) {
	t.Parallel()

	ctx := ContextWithTraceID(context.Background(), "trace-2")

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.ComponentSpan(ctx, "auth-service").Info("component log")

	m := decodeLogLine(t, &buf)
	if got, _ := m[FieldComponent].(string); got != "auth-service" {
		t.Errorf("component = %q, want %q", got, "auth-service")
	}
	if got, _ := m[FieldTraceID].(string); got != "trace-2" {
		t.Errorf("trace_id = %q, want %q", got, "trace-2")
	}
}

func TestRequestSpan(t *testing.T) {
	t.Parallel()

	ctx := ContextWithCorrelationID(context.Background(), "corr-2")

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.RequestSpan(ctx, "GET", "/v1/users", "req-2").Info("request log")

	m := decodeLogLine(t, &buf)
	if got, _ := m[FieldHTTPMethod].(string); got != "GET" {
		t.Errorf("http.method = %q, want %q", got, "GET")
	}
	if got, _ := m[FieldHTTPPath].(string); got != "/v1/users" {
		t.Errorf("http.path = %q, want %q", got, "/v1/users")
	}
	if got, _ := m[FieldRequestID].(string); got != "req-2" {
		t.Errorf("request_id = %q, want %q", got, "req-2")
	}
	if got, _ := m[FieldCorrelationID].(string); got != "corr-2" {
		t.Errorf("correlation_id = %q, want %q", got, "corr-2")
	}
}

func TestContextWithLoggerRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	want := newBufferedLogger(&buf, "info", "json")

	ctx := ContextWithLogger(context.Background(), want)
	got, ok := LoggerFromContext(ctx)
	if !ok {
		t.Fatal("LoggerFromContext returned ok=false for an injected logger")
	}
	if got != want {
		t.Errorf("LoggerFromContext returned a different logger than was injected")
	}
}

func TestContextWithLoggerNilIsNotStored(t *testing.T) {
	t.Parallel()

	ctx := ContextWithLogger(context.Background(), nil)
	if _, ok := LoggerFromContext(ctx); ok {
		t.Error("LoggerFromContext reported ok=true after injecting a nil logger")
	}
}

func TestLoggerFromContextEmpty(t *testing.T) {
	t.Parallel()

	if log, ok := LoggerFromContext(context.Background()); ok || log != nil {
		t.Errorf("LoggerFromContext(empty) = (%v, %v), want (nil, false)", log, ok)
	}
}
