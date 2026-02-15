package logger

import (
	"context"
	"os"
	"testing"
)

func TestNewDefault(t *testing.T) {
	l := NewDefault("test-svc")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.service != "test-svc" {
		t.Errorf("expected service 'test-svc', got %q", l.service)
	}
}

func TestNew(t *testing.T) {
	cfg := &Config{
		Level:  "debug",
		Format: "json",
		Output: "stdout",
	}
	l := New(cfg, "my-service")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.service != "my-service" {
		t.Errorf("expected service 'my-service', got %q", l.service)
	}
}

func TestNewInvalidLevel(t *testing.T) {
	cfg := &Config{
		Level:  "invalid-level",
		Format: "json",
		Output: "stdout",
	}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected logger to be created even with invalid level")
	}
}

func TestNewFromEnv(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOG_FORMAT", "json")
	defer os.Unsetenv("LOG_LEVEL")
	defer os.Unsetenv("LOG_FORMAT")

	l := NewFromEnv("env-svc")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestWithComponent(t *testing.T) {
	l := NewDefault("test")
	cl := l.WithComponent("handler")
	if cl == nil {
		t.Fatal("expected non-nil logger")
	}
	if cl.service != "test" {
		t.Errorf("service should be preserved, got %q", cl.service)
	}
}

func TestWithContext(t *testing.T) {
	l := NewDefault("test")
	ctx := context.WithValue(context.Background(), contextKey("trace_id"), "abc123")
	cl := l.WithContext(ctx)
	if cl == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestWithFields(t *testing.T) {
	l := NewDefault("test")
	fl := l.WithFields(map[string]interface{}{"key": "value"})
	if fl == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestWithError(t *testing.T) {
	l := NewDefault("test")
	el := l.WithError(nil)
	if el == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestInit(t *testing.T) {
	cfg := Config{
		Level:  "info",
		Format: "console",
		Output: "stdout",
	}
	Init(cfg)
	gl := GetGlobalLogger()
	if gl == nil {
		t.Fatal("expected global logger to be set after Init")
	}
}

func TestGetGlobalLoggerDefault(t *testing.T) {
	globalLogger = nil
	l := GetGlobalLogger()
	if l == nil {
		t.Fatal("expected default global logger to be created")
	}
}

func TestSetGlobalLogger(t *testing.T) {
	l := NewDefault("custom")
	SetGlobalLogger(l)
	got := GetGlobalLogger()
	if got != l {
		t.Error("expected SetGlobalLogger to set the global logger")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	Init(Config{Level: "debug", Format: "console", Output: "stdout"})
	// These should not panic
	Debug("debug msg")
	Info("info msg")
	Warn("warn msg")
	Error("error msg")
}

func TestConfigApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.Level != "info" {
		t.Errorf("expected level 'info', got %q", cfg.Level)
	}
	if cfg.Format != "console" {
		t.Errorf("expected format 'console', got %q", cfg.Format)
	}
	if cfg.Output != "stdout" {
		t.Errorf("expected output 'stdout', got %q", cfg.Output)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("expected MaxSize 100, got %d", cfg.MaxSize)
	}
	if cfg.MaxBackups != 3 {
		t.Errorf("expected MaxBackups 3, got %d", cfg.MaxBackups)
	}
	if cfg.MaxAge != 28 {
		t.Errorf("expected MaxAge 28, got %d", cfg.MaxAge)
	}
	if !cfg.Timestamp {
		t.Error("expected Timestamp to be true")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{Level: "info", Format: "json"}, false},
		{"valid console", Config{Level: "debug", Format: "console"}, false},
		{"invalid level", Config{Level: "bad", Format: "json"}, true},
		{"invalid format", Config{Level: "info", Format: "xml"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConsoleLoggerFormat(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		NoColor: true,
	}
	l := New(cfg, "test-svc")
	if l == nil {
		t.Fatal("expected logger with console format")
	}
}
