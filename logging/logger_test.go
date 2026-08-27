package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("unmarshal %q: %v", buf.String(), err)
	}
	return m
}

func TestNew_ReturnsLoggerWithoutError(t *testing.T) {
	t.Parallel()

	l, err := New(&Config{Level: "info", Format: "json", Output: "stdout"}, "svc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l == nil || l.Slog() == nil {
		t.Fatal("expected a usable logger")
	}
	if err := l.Close(); err != nil {
		t.Errorf("Close without OTLP should be a no-op, got %v", err)
	}
}

func TestNewDefault(t *testing.T) {
	t.Parallel()

	l := NewDefault("svc")
	if l == nil || l.Slog() == nil {
		t.Fatal("expected a usable default logger")
	}
	if l.Level() != slog.LevelInfo {
		t.Errorf("default level = %v, want info", l.Level())
	}
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FORMAT", "json")

	l, err := NewFromEnv("svc")
	if err != nil {
		t.Fatalf("NewFromEnv: %v", err)
	}
	if l.Level() != slog.LevelWarn {
		t.Errorf("level = %v, want warn", l.Level())
	}
}

func TestLevelsEmitAndRenderSchema(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.Info("hello", map[string]any{"k": "v"})

	m := decodeLine(t, &buf)
	if m[FieldMessage] != "hello" {
		t.Errorf("message = %v, want hello", m[FieldMessage])
	}
	if m[FieldLevel] != "INFO" {
		t.Errorf("level = %v, want INFO", m[FieldLevel])
	}
	if _, ok := m[FieldTimestamp]; !ok {
		t.Error("expected timestamp field")
	}
	if m[FieldService] != "test" {
		t.Errorf("service = %v, want test", m[FieldService])
	}
	if m["k"] != "v" {
		t.Errorf("field k = %v, want v", m["k"])
	}
}

func TestLevelFilteringDropsBelowBase(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "warn", "json")
	l.Info("dropped")
	l.Warn("kept")

	lines := decodeLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d (%q)", len(lines), buf.String())
	}
	if lines[0][FieldMessage] != "kept" {
		t.Errorf("message = %v, want kept", lines[0][FieldMessage])
	}
}

func TestSetLevelDynamic(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")

	l.Debug("before")
	if buf.Len() != 0 {
		t.Fatalf("debug should be filtered at info, got %q", buf.String())
	}

	l.SetLevel("debug")
	if l.Level() != slog.LevelDebug {
		t.Fatalf("level = %v, want debug", l.Level())
	}
	l.Debug("after")
	if !strings.Contains(buf.String(), "after") {
		t.Errorf("debug should pass after SetLevel(debug), got %q", buf.String())
	}
}

func TestTraceLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "trace", "json")
	l.Trace("deep")

	m := decodeLine(t, &buf)
	if m[FieldLevel] != "TRACE" {
		t.Errorf("level = %v, want TRACE", m[FieldLevel])
	}
}

func TestWithComponentAndFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.WithComponent("auth").WithFields(map[string]any{"tenant": "acme"}).Info("hi")

	m := decodeLine(t, &buf)
	if m[FieldComponent] != "auth" {
		t.Errorf("component = %v, want auth", m[FieldComponent])
	}
	if m["tenant"] != "acme" {
		t.Errorf("tenant = %v, want acme", m["tenant"])
	}
}

func TestWithError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.WithError(context.Canceled).Error("boom")

	m := decodeLine(t, &buf)
	if m[FieldError] != context.Canceled.Error() {
		t.Errorf("error = %v, want %v", m[FieldError], context.Canceled)
	}
}

func TestWithErrorNilIsNoOp(t *testing.T) {
	t.Parallel()

	l := NewDefault("svc")
	if l.WithError(nil) != l {
		t.Error("WithError(nil) should return the same logger")
	}
}

func TestSlogEscapeHatchSharesPipeline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "info", "json")
	l.Slog().Info("via slog", slog.String("k", "v"))

	m := decodeLine(t, &buf)
	if m[FieldMessage] != "via slog" || m["k"] != "v" {
		t.Errorf("slog escape hatch did not use the pipeline: %v", m)
	}
}

func TestWithHandlerFansOutAndIsGoverned(t *testing.T) {
	t.Parallel()

	var primary, byo bytes.Buffer
	cfg := &Config{
		Level:   "info",
		Format:  "json",
		Output:  "stdout",
		Masking: MaskingConfig{Enabled: true, Replacement: "***"},
	}
	byoHandler := slog.NewJSONHandler(&byo, nil)
	l, err := New(cfg, "svc", WithWriter(&primary), WithHandler(byoHandler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	l.Info("login", map[string]any{"password": "secret"})

	for name, b := range map[string]*bytes.Buffer{"primary": &primary, "byo": &byo} {
		if !strings.Contains(b.String(), "login") {
			t.Errorf("%s sink missing record: %q", name, b.String())
		}
		if strings.Contains(b.String(), "secret") {
			t.Errorf("%s sink leaked secret: %q", name, b.String())
		}
	}
}

func TestWithBaseSinkReplacesDefault(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	l, err := New(&Config{Level: "info", Format: "json"}, "svc", WithBaseSink(base))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("custom")

	// The consumer-owned sink uses stdlib keys, proving it replaced the default.
	m := decodeLine(t, &buf)
	if m["msg"] != "custom" {
		t.Errorf("expected stdlib msg key from base sink, got %v", m)
	}
}
