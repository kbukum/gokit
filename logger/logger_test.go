package logger

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
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
	Init(&cfg)
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
	Init(&Config{Level: "debug", Format: "console", Output: "stdout"})
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
		Level:   "info",
		Format:  "console",
		Output:  "stdout",
		NoColor: true,
	}
	l := New(cfg, "test-svc")
	if l == nil {
		t.Fatal("expected logger with console format")
	}
}

func TestGetLoggerZ(t *testing.T) {
	Init(&Config{Level: "info", Format: "json", Output: "stdout"})
	zl := GetLoggerZ()
	// zerolog.Logger is a struct - just verify it doesn't panic
	_ = zl
}

func TestGetLoggerMethod(t *testing.T) {
	l := NewDefault("test")
	zl := l.GetLogger()
	_ = zl
}

func TestRegisterAndGet(t *testing.T) {
	l := NewDefault("custom-component")
	Register("my-component", l)

	got := Get("my-component")
	if got != l {
		t.Error("expected Get to return the registered logger")
	}
}

func TestGetUnregistered(t *testing.T) {
	// Getting an unregistered name should return global logger with component tag
	got := Get("unregistered-component")
	if got == nil {
		t.Fatal("expected non-nil logger for unregistered component")
	}
}

func TestRegisterDefaults(t *testing.T) {
	Init(&Config{Level: "info", Format: "json", Output: "stdout"})
	RegisterDefaults("handler", "repository", "service")

	for _, name := range []string{"handler", "repository", "service"} {
		got := Get(name)
		if got == nil {
			t.Errorf("expected non-nil logger for %q", name)
		}
	}
}

func TestFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected map[string]interface{}
	}{
		{
			"key-value pairs",
			[]interface{}{"op", "save", "id", 42},
			map[string]interface{}{"op": "save", "id": 42},
		},
		{
			"odd number of args",
			[]interface{}{"op", "save", "trailing"},
			map[string]interface{}{"op": "save"},
		},
		{
			"empty",
			[]interface{}{},
			map[string]interface{}{},
		},
		{
			"non-string key skipped",
			[]interface{}{123, "value", "key", "val"},
			map[string]interface{}{"key": "val"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Fields(tc.input...)
			for k, v := range tc.expected {
				if result[k] != v {
					t.Errorf("Fields[%q] = %v, expected %v", k, result[k], v)
				}
			}
		})
	}
}

func TestErrorFields(t *testing.T) {
	err := fmt.Errorf("something broke")
	fields := ErrorFields("create-user", err)

	if fields[FieldOperation] != "create-user" {
		t.Errorf("expected operation 'create-user', got %v", fields[FieldOperation])
	}
	if fields[FieldError] != "something broke" {
		t.Errorf("expected error 'something broke', got %v", fields[FieldError])
	}
}

func TestDurationFields(t *testing.T) {
	d := 150 * time.Millisecond
	fields := DurationFields("query", d)

	if fields[FieldOperation] != "query" {
		t.Errorf("expected operation 'query', got %v", fields[FieldOperation])
	}
	if fields[FieldDuration] != int64(150) {
		t.Errorf("expected duration 150, got %v", fields[FieldDuration])
	}
}

func TestMergeWithError(t *testing.T) {
	err := fmt.Errorf("test error")

	// Merge into existing map
	fields := map[string]interface{}{"op": "save"}
	result := MergeWithError(fields, err)
	if result[FieldError] != "test error" {
		t.Errorf("expected error field, got %v", result[FieldError])
	}
	if result["op"] != "save" {
		t.Error("expected existing fields to be preserved")
	}

	// Merge into nil map
	result2 := MergeWithError(nil, err)
	if result2[FieldError] != "test error" {
		t.Errorf("expected error field from nil map, got %v", result2[FieldError])
	}
}

func TestMergeWithDuration(t *testing.T) {
	d := 200 * time.Millisecond

	// Merge into existing map
	fields := map[string]interface{}{"op": "query"}
	result := MergeWithDuration(fields, d)
	if result[FieldDuration] != int64(200) {
		t.Errorf("expected duration 200, got %v", result[FieldDuration])
	}
	if result["op"] != "query" {
		t.Error("expected existing fields to be preserved")
	}

	// Merge into nil map
	result2 := MergeWithDuration(nil, d)
	if result2[FieldDuration] != int64(200) {
		t.Errorf("expected duration from nil map, got %v", result2[FieldDuration])
	}
}

func TestNewWithStderrOutput(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "json",
		Output: "stderr",
	}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected non-nil logger with stderr output")
	}
}

func TestNewWithPrettyFormat(t *testing.T) {
	cfg := &Config{
		Level:  "info",
		Format: "pretty",
		Output: "stdout",
	}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected non-nil logger with pretty format")
	}
}

func TestComponentRegistry(t *testing.T) {
	cr := NewComponentRegistry()
	if cr == nil {
		t.Fatal("expected non-nil registry")
	}

	cr.SetAPIPrefix("/api/v1/")
	if cr.APIPrefix() != "/api/v1" {
		t.Errorf("expected '/api/v1', got %q", cr.APIPrefix())
	}

	if cr.StartTime().IsZero() {
		t.Error("expected non-zero start time")
	}

	cr.RegisterInfrastructure("PostgreSQL", "database", "active", "localhost:5432")
	if len(cr.Infrastructure()) != 1 {
		t.Errorf("expected 1 infrastructure, got %d", len(cr.Infrastructure()))
	}
	if cr.Infrastructure()[0].Name != "PostgreSQL" {
		t.Errorf("expected name 'PostgreSQL', got %q", cr.Infrastructure()[0].Name)
	}

	cr.RegisterService("user-service", "active", []string{"db", "cache"})
	if len(cr.Services()) != 1 {
		t.Errorf("expected 1 service, got %d", len(cr.Services()))
	}

	cr.RegisterRepository("user-repo", "PostgreSQL", "active")
	if len(cr.Repositories()) != 1 {
		t.Errorf("expected 1 repository, got %d", len(cr.Repositories()))
	}

	cr.RegisterClient("auth-client", "gRPC:9090", "connected")
	if len(cr.Clients()) != 1 {
		t.Errorf("expected 1 client, got %d", len(cr.Clients()))
	}

	cr.RegisterHandler("GET", "/users", "UserHandler")
	if len(cr.Handlers()) != 1 {
		t.Errorf("expected 1 handler, got %d", len(cr.Handlers()))
	}

	cr.SetHandlers([]HandlerComponent{
		{Method: "POST", Path: "/users", Handler: "CreateUser"},
	})
	if len(cr.Handlers()) != 1 || cr.Handlers()[0].Method != "POST" {
		t.Error("expected SetHandlers to replace handler list")
	}

	cr.RegisterConsumer("order-consumer", "group-1", "orders", 3, "active")
	if len(cr.Consumers()) != 1 {
		t.Errorf("expected 1 consumer, got %d", len(cr.Consumers()))
	}
}

func TestWithContextAllKeys(t *testing.T) {
	l := NewDefault("test")
	ctx := context.Background()
	ctx = context.WithValue(ctx, contextKey("trace_id"), "trace-123")
	ctx = context.WithValue(ctx, contextKey("span_id"), "span-456")
	ctx = context.WithValue(ctx, contextKey("request_id"), "req-789")
	ctx = context.WithValue(ctx, contextKey("user_id"), "user-1")
	ctx = context.WithValue(ctx, contextKey("correlation_id"), "corr-2")

	cl := l.WithContext(ctx)
	if cl == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewFromEnvDefaults(t *testing.T) {
	// Clear env vars to use defaults
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("LOG_FORMAT")
	os.Unsetenv("LOG_OUTPUT")
	os.Unsetenv("LOG_NO_COLOR")
	os.Unsetenv("LOG_TIMESTAMP")

	l := NewFromEnv("defaults-svc")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestPackageLevelWithContext(t *testing.T) {
	Init(&Config{Level: "debug", Format: "json", Output: "stdout"})
	ctx := context.Background()
	l := WithContext(ctx)
	if l == nil {
		t.Fatal("expected non-nil logger from WithContext")
	}
}

func TestPackageLevelWithComponent(t *testing.T) {
	Init(&Config{Level: "debug", Format: "json", Output: "stdout"})
	l := WithComponent("handler")
	if l == nil {
		t.Fatal("expected non-nil logger from WithComponent")
	}
}

func TestInitWithConsoleFormat(t *testing.T) {
	cfg := Config{
		Level:       "debug",
		Format:      "console",
		Output:      "stdout",
		ServiceName: "init-test",
	}
	Init(&cfg)
	gl := GetGlobalLogger()
	if gl == nil {
		t.Fatal("expected global logger after Init")
	}
}
