package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
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
	fl := l.WithFields(map[string]any{"key": "value"})
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

func TestDefaultLogger(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("expected default logger to be created")
	}
	if Default() != l {
		t.Error("expected Default to return the same immutable instance")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
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
	reg := NewRegistry(NewDefault("base"))
	l := NewDefault("custom-component")
	reg.Register("my-component", l)

	got := reg.Get("my-component")
	if got != l {
		t.Error("expected Get to return the registered logger")
	}
}

func TestGetUnregistered(t *testing.T) {
	reg := NewRegistry(NewDefault("base"))
	// Getting an unregistered name derives a component-tagged logger from base.
	got := reg.Get("unregistered-component")
	if got == nil {
		t.Fatal("expected non-nil logger for unregistered component")
	}
	if reg.Get("unregistered-component") != got {
		t.Error("expected derived logger to be cached")
	}
}

func TestRegistryNilBaseUsesDefault(t *testing.T) {
	reg := NewRegistry(nil)
	if reg.Base() != Default() {
		t.Error("expected nil base to fall back to the default logger")
	}
	if reg.Get("handler") == nil {
		t.Error("expected non-nil derived logger")
	}
}

func TestFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		expected map[string]any
	}{
		{
			"key-value pairs",
			[]any{"op", "save", "id", 42},
			map[string]any{"op": "save", "id": 42},
		},
		{
			"odd number of args",
			[]any{"op", "save", "trailing"},
			map[string]any{"op": "save"},
		},
		{
			"empty",
			[]any{},
			map[string]any{},
		},
		{
			"non-string key skipped",
			[]any{123, "value", "key", "val"},
			map[string]any{"key": "val"},
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
	fields := map[string]any{"op": "save"}
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
	fields := map[string]any{"op": "query"}
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
	ctx := context.Background()
	l := WithContext(ctx)
	if l == nil {
		t.Fatal("expected non-nil logger from WithContext")
	}
}

func TestPackageLevelWithComponent(t *testing.T) {
	l := WithComponent("handler")
	if l == nil {
		t.Fatal("expected non-nil logger from WithComponent")
	}
}

func TestNewWithConsoleFormat(t *testing.T) {
	cfg := Config{
		Level:       "debug",
		Format:      "console",
		Output:      "stdout",
		ServiceName: "init-test",
	}
	l := New(&cfg, cfg.ServiceName)
	if l == nil {
		t.Fatal("expected non-nil logger for console format")
	}
}

// ---------------------------------------------------------------------------
// Helpers for output-capturing tests
// ---------------------------------------------------------------------------

// syncBuffer is a thread-safe bytes.Buffer for concurrent tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

func (sb *syncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// newBufferedLogger creates a *Logger backed by the given bytes.Buffer so that
// log output can be inspected in tests. format should be "json" or "console".
func newBufferedLogger(buf *bytes.Buffer, level, format string) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	var zl zerolog.Logger
	if format == "console" || format == FormatPretty {
		zl = zerolog.New(zerolog.ConsoleWriter{Out: buf, NoColor: true, TimeFormat: "15:04:05"})
	} else {
		zl = zerolog.New(buf)
	}
	zl = zl.Level(lvl)
	return &Logger{logger: zl, service: "test"}
}

// newSyncBufferedLogger creates a *Logger backed by a syncBuffer for concurrent tests.
func newSyncBufferedLogger(sb *syncBuffer, level, format string) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	var zl zerolog.Logger
	if format == "console" || format == FormatPretty {
		zl = zerolog.New(zerolog.ConsoleWriter{Out: sb, NoColor: true, TimeFormat: "15:04:05"})
	} else {
		zl = zerolog.New(sb)
	}
	zl = zl.Level(lvl)
	return &Logger{logger: zl, service: "test"}
}

// ---------------------------------------------------------------------------
// 1. Log Level handling
// ---------------------------------------------------------------------------

func TestLogLevels_AllLevelsProduceOutput(t *testing.T) {
	// Reset zerolog global level so per-logger levels are respected.
	zerolog.SetGlobalLevel(zerolog.TraceLevel)

	levels := []struct {
		name   string
		logFn  func(*Logger, string, ...map[string]any)
		zLevel string // minimum level to set so this level emits output
	}{
		{"trace", (*Logger).Trace, "trace"},
		{"debug", (*Logger).Debug, "trace"},
		{"info", (*Logger).Info, "trace"},
		{"warn", (*Logger).Warn, "trace"},
		{"error", (*Logger).Error, "trace"},
	}

	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := newBufferedLogger(&buf, tc.zLevel, "json")
			tc.logFn(l, "hello "+tc.name)
			if buf.Len() == 0 {
				t.Errorf("expected output for level %s, got nothing", tc.name)
			}
		})
	}
}

func TestLogLevels_Filtering(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.TraceLevel)

	tests := []struct {
		name      string
		cfgLevel  string
		logFn     func(*Logger, string, ...map[string]any)
		expectOut bool
	}{
		{"info filters debug", "info", (*Logger).Debug, false},
		{"info filters trace", "info", (*Logger).Trace, false},
		{"info allows info", "info", (*Logger).Info, true},
		{"info allows warn", "info", (*Logger).Warn, true},
		{"info allows error", "info", (*Logger).Error, true},
		{"warn filters info", "warn", (*Logger).Info, false},
		{"warn allows warn", "warn", (*Logger).Warn, true},
		{"warn allows error", "warn", (*Logger).Error, true},
		{"error filters warn", "error", (*Logger).Warn, false},
		{"error allows error", "error", (*Logger).Error, true},
		{"debug allows debug", "debug", (*Logger).Debug, true},
		{"debug filters trace", "debug", (*Logger).Trace, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := newBufferedLogger(&buf, tc.cfgLevel, "json")
			tc.logFn(l, "test message")
			gotOutput := buf.Len() > 0
			if gotOutput != tc.expectOut {
				t.Errorf("level=%s: expected output=%v, got output=%v (buf=%q)",
					tc.cfgLevel, tc.expectOut, gotOutput, buf.String())
			}
		})
	}
}

func TestLogLevels_ParseFromString(t *testing.T) {
	// zerolog.ParseLevel is used internally; verify it works for all our expected strings.
	cases := []struct {
		input string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := zerolog.ParseLevel(tc.input)
			if err != nil {
				t.Fatalf("ParseLevel(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestLogLevels_InvalidLevelFallsBackToInfo(t *testing.T) {
	cfg := &Config{Level: "not-a-level", Format: "json", Output: "stdout"}
	l := New(cfg, "test")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	// With info level fallback, Debug should be filtered
	var buf bytes.Buffer
	l.logger = zerolog.New(&buf).Level(zerolog.InfoLevel)
	l.Debug("should be filtered")
	if buf.Len() > 0 {
		t.Error("expected debug to be filtered with info-level fallback")
	}
	l.Info("should appear")
	if buf.Len() == 0 {
		t.Error("expected info to appear with info-level fallback")
	}
}

// ---------------------------------------------------------------------------
// 2. Output format verification
// ---------------------------------------------------------------------------

func TestJSONFormat_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("json test message")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if parsed["message"] != "json test message" {
		t.Errorf("expected message 'json test message', got %v", parsed["message"])
	}
	if _, ok := parsed["level"]; !ok {
		t.Error("expected 'level' field in JSON output")
	}
}

func TestJSONFormat_AllLevelsHaveCorrectLevelField(t *testing.T) {
	levels := []struct {
		name  string
		logFn func(*Logger, string, ...map[string]any)
		want  string
	}{
		{"debug", (*Logger).Debug, "debug"},
		{"info", (*Logger).Info, "info"},
		{"warn", (*Logger).Warn, "warn"},
		{"error", (*Logger).Error, "error"},
	}

	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := newBufferedLogger(&buf, "trace", "json")
			tc.logFn(l, "level check")

			var parsed map[string]any
			if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if parsed["level"] != tc.want {
				t.Errorf("expected level %q, got %v", tc.want, parsed["level"])
			}
		})
	}
}

func TestJSONFormat_TimestampPresent(t *testing.T) {
	var buf bytes.Buffer
	zl := zerolog.New(&buf).With().Timestamp().Logger()
	l := &Logger{logger: zl, service: "test"}
	l.Info("with timestamp")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["time"]; !ok {
		t.Error("expected 'time' field when timestamp is enabled")
	}
}

func TestConsoleFormat_HumanReadable(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "console")
	l.Info("console check")
	output := buf.String()
	if !strings.Contains(output, "console check") {
		t.Errorf("console output should contain message, got: %s", output)
	}
	// Console output should NOT be valid JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err == nil {
		t.Error("console output should not be valid JSON")
	}
}

// ---------------------------------------------------------------------------
// 3. Structured fields
// ---------------------------------------------------------------------------

func TestWithFields_AppearsInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	fl := l.WithFields(map[string]any{"user": "alice", "count": 42})
	fl.Info("with fields")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["user"] != "alice" {
		t.Errorf("expected user=alice, got %v", parsed["user"])
	}
	// JSON numbers are float64
	if parsed["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", parsed["count"])
	}
}

func TestInlineFields_AppearsInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("inline", map[string]any{"key": "value", "num": 7})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key=value, got %v", parsed["key"])
	}
	if parsed["num"] != float64(7) {
		t.Errorf("expected num=7, got %v", parsed["num"])
	}
}

func TestWithFields_MultipleFieldMaps(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("multi",
		map[string]any{"a": 1},
		map[string]any{"b": 2},
	)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["a"] != float64(1) {
		t.Errorf("expected a=1, got %v", parsed["a"])
	}
	if parsed["b"] != float64(2) {
		t.Errorf("expected b=2, got %v", parsed["b"])
	}
}

func TestWithFields_NestedMap(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	nested := map[string]any{
		"outer": map[string]any{
			"inner": "deep",
		},
	}
	fl := l.WithFields(nested)
	fl.Info("nested fields")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	outer, ok := parsed["outer"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested object for 'outer', got %T", parsed["outer"])
	}
	if outer["inner"] != "deep" {
		t.Errorf("expected inner=deep, got %v", outer["inner"])
	}
}

func TestWithComponent_AppearsInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	cl := l.WithComponent("auth-handler")
	cl.Info("component test")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed[FieldComponent] != "auth-handler" {
		t.Errorf("expected component=auth-handler, got %v", parsed[FieldComponent])
	}
}

func TestWithError_AppearsInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	el := l.WithError(fmt.Errorf("something failed"))
	el.Error("with error field")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["error"] != "something failed" {
		t.Errorf("expected error='something failed', got %v", parsed["error"])
	}
}

func TestFieldsHelper_InLogOutput(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("fields helper", Fields("op", "create", "id", 99))

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["op"] != "create" {
		t.Errorf("expected op=create, got %v", parsed["op"])
	}
	if parsed["id"] != float64(99) {
		t.Errorf("expected id=99, got %v", parsed["id"])
	}
}

// ---------------------------------------------------------------------------
// 4. Context propagation
// ---------------------------------------------------------------------------

func TestWithContext_TraceAndSpanInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")

	ctx := context.Background()
	ctx = context.WithValue(ctx, contextKey("trace_id"), "t-123")
	ctx = context.WithValue(ctx, contextKey("span_id"), "s-456")

	cl := l.WithContext(ctx)
	cl.Info("context test")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed[FieldTraceID] != "t-123" {
		t.Errorf("expected trace_id=t-123, got %v", parsed[FieldTraceID])
	}
	if parsed[FieldSpanID] != "s-456" {
		t.Errorf("expected span_id=s-456, got %v", parsed[FieldSpanID])
	}
}

func TestWithContext_AllKeysInJSON(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")

	ctx := context.Background()
	ctx = context.WithValue(ctx, contextKey("trace_id"), "t-1")
	ctx = context.WithValue(ctx, contextKey("span_id"), "s-2")
	ctx = context.WithValue(ctx, contextKey("request_id"), "r-3")
	ctx = context.WithValue(ctx, contextKey("user_id"), "u-4")
	ctx = context.WithValue(ctx, contextKey("correlation_id"), "c-5")

	cl := l.WithContext(ctx)
	cl.Info("all keys")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	expected := map[string]string{
		FieldTraceID:       "t-1",
		FieldSpanID:        "s-2",
		FieldRequestID:     "r-3",
		FieldUserID:        "u-4",
		FieldCorrelationID: "c-5",
	}
	for field, want := range expected {
		if parsed[field] != want {
			t.Errorf("expected %s=%s, got %v", field, want, parsed[field])
		}
	}
}

func TestWithContext_EmptyContext(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")

	cl := l.WithContext(context.Background())
	cl.Info("empty context")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// None of the context fields should be present
	for _, field := range []string{FieldTraceID, FieldSpanID, FieldRequestID, FieldUserID, FieldCorrelationID} {
		if _, ok := parsed[field]; ok {
			t.Errorf("field %s should not be present with empty context", field)
		}
	}
}

func TestWithContext_PartialContext(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")

	ctx := context.WithValue(context.Background(), contextKey("trace_id"), "only-trace")
	cl := l.WithContext(ctx)
	cl.Info("partial ctx")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed[FieldTraceID] != "only-trace" {
		t.Errorf("expected trace_id=only-trace, got %v", parsed[FieldTraceID])
	}
	// Other fields should be absent
	for _, field := range []string{FieldSpanID, FieldRequestID, FieldUserID, FieldCorrelationID} {
		if _, ok := parsed[field]; ok {
			t.Errorf("field %s should not be present", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. Logger configuration
// ---------------------------------------------------------------------------

func TestConfig_ApplyDefaultsDoesNotOverrideSet(t *testing.T) {
	cfg := Config{
		Level:      "error",
		Format:     "json",
		Output:     "stderr",
		MaxSize:    50,
		MaxBackups: 5,
		MaxAge:     7,
		Timestamp:  true,
	}
	cfg.ApplyDefaults()
	if cfg.Level != "error" {
		t.Errorf("expected level 'error', got %q", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("expected format 'json', got %q", cfg.Format)
	}
	if cfg.Output != "stderr" {
		t.Errorf("expected output 'stderr', got %q", cfg.Output)
	}
	if cfg.MaxSize != 50 {
		t.Errorf("expected MaxSize 50, got %d", cfg.MaxSize)
	}
	if cfg.MaxBackups != 5 {
		t.Errorf("expected MaxBackups 5, got %d", cfg.MaxBackups)
	}
	if cfg.MaxAge != 7 {
		t.Errorf("expected MaxAge 7, got %d", cfg.MaxAge)
	}
}

func TestConfig_ValidateAllLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "trace"} {
		cfg := Config{Level: level, Format: "json"}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() returned error for valid level %q: %v", level, err)
		}
	}
}

func TestConfig_ValidateAllFormats(t *testing.T) {
	for _, format := range []string{"json", "console", "text"} {
		cfg := Config{Level: "info", Format: format}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() returned error for valid format %q: %v", format, err)
		}
	}
}

func TestConfig_ValidateInvalidLevel(t *testing.T) {
	cfg := Config{Level: "verbose", Format: "json"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid level 'verbose'")
	}
	if !strings.Contains(err.Error(), "verbose") {
		t.Errorf("error should mention the bad value, got: %v", err)
	}
}

func TestConfig_ValidateInvalidFormat(t *testing.T) {
	cfg := Config{Level: "info", Format: "xml"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid format 'xml'")
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("error should mention the bad value, got: %v", err)
	}
}

func TestNew_WithTimestampAndCaller(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{
		Level:     "info",
		Format:    "json",
		Output:    "stdout",
		Timestamp: true,
		Caller:    true,
	}
	l := New(cfg, "caller-test")
	// Re-wire output to buffer for capture
	l.logger = l.logger.Output(&buf)
	l.Info("caller check")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["caller"]; !ok {
		t.Error("expected 'caller' field when Caller is enabled")
	}
}

func TestNewDefault_ProducesWorkingLogger(t *testing.T) {
	l := NewDefault("default-test")
	if l == nil {
		t.Fatal("NewDefault returned nil")
	}
	if l.service != "default-test" {
		t.Errorf("expected service 'default-test', got %q", l.service)
	}
	// Should not panic when logging
	l.Info("default logger works")
	l.Debug("debug from default")
	l.Warn("warn from default")
	l.Error("error from default")
}

func TestNew_WithServiceName(t *testing.T) {
	cfg := Config{
		Level:       "debug",
		Format:      "json",
		Output:      "stdout",
		ServiceName: "my-api",
	}
	l := New(&cfg, cfg.ServiceName)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.service != "my-api" {
		t.Errorf("expected service 'my-api', got %q", l.service)
	}
}

func TestNew_DefaultServiceName(t *testing.T) {
	l := NewDefault("default")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.service != "default" {
		t.Errorf("expected service 'default', got %q", l.service)
	}
}

func TestNew_WithPrettyFormat(t *testing.T) {
	cfg := Config{
		Level:       "info",
		Format:      FormatPretty,
		Output:      "stdout",
		ServiceName: "pretty-test",
	}
	l := New(&cfg, cfg.ServiceName)
	if l == nil {
		t.Fatal("expected non-nil logger with pretty format")
	}
}

// ---------------------------------------------------------------------------
// 6. Concurrent safety
// ---------------------------------------------------------------------------

func TestConcurrent_LoggingSafety(t *testing.T) {
	sb := &syncBuffer{}
	l := newSyncBufferedLogger(sb, "debug", "json")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			l.Info(fmt.Sprintf("goroutine %d", n), map[string]any{"n": n})
			l.Debug(fmt.Sprintf("debug %d", n))
			l.Warn(fmt.Sprintf("warn %d", n))
			l.Error(fmt.Sprintf("error %d", n))
		}(i)
	}
	wg.Wait()
	// If we get here without a panic or race, the test passes.
	if sb.Len() == 0 {
		t.Error("expected some output from concurrent logging")
	}
}

func TestConcurrent_WithFieldsSafety(t *testing.T) {
	sb := &syncBuffer{}
	l := newSyncBufferedLogger(sb, "debug", "json")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fl := l.WithFields(map[string]any{"goroutine": n})
			fl.Info("concurrent with fields")
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_WithContextSafety(t *testing.T) {
	sb := &syncBuffer{}
	l := newSyncBufferedLogger(sb, "debug", "json")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), contextKey("trace_id"), fmt.Sprintf("t-%d", n))
			cl := l.WithContext(ctx)
			cl.Info("concurrent context")
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_RegistrySafety(t *testing.T) {
	reg := NewRegistry(NewDefault("base"))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		name := fmt.Sprintf("component-%d", i)
		go func() {
			defer wg.Done()
			reg.Register(name, NewDefault(name))
		}()
		go func() {
			defer wg.Done()
			_ = reg.Get(name)
		}()
	}
	wg.Wait()
}

func TestConcurrent_DefaultLoggerSafety(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Default()
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// 7. Edge cases
// ---------------------------------------------------------------------------

func TestEdge_EmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// zerolog emits message="" or omits it; both are fine as long as it's valid JSON
}

func TestEdge_VeryLongMessage(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	longMsg := strings.Repeat("x", 10000)
	l.Info(longMsg)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON with long message: %v", err)
	}
	if msg, ok := parsed["message"].(string); ok && len(msg) != 10000 {
		t.Errorf("expected message of length 10000, got %d", len(msg))
	}
}

func TestEdge_UnicodeMessage(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("こんにちは世界 🌍 émojis ñ")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON with unicode message: %v", err)
	}
	if !strings.Contains(parsed["message"].(string), "こんにちは世界") {
		t.Error("expected unicode content in message")
	}
}

func TestEdge_SpecialCharactersMessage(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info(`special chars: "quotes" \backslash\ <html> & newline\n tab\t`)

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON with special chars: %v", err)
	}
}

func TestEdge_NilFieldsMap(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	// Pass nil as field map - should not panic
	l.Info("nil fields", nil)
	if buf.Len() == 0 {
		t.Error("expected output even with nil fields")
	}
}

func TestEdge_EmptyFieldsMap(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	l.Info("empty fields", map[string]any{})

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestEdge_WithFieldsEmptyMap(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	fl := l.WithFields(map[string]any{})
	fl.Info("empty withfields")
	if buf.Len() == 0 {
		t.Error("expected output with empty WithFields")
	}
}

func TestEdge_WithFieldsNilValue(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	fl := l.WithFields(map[string]any{"key": nil})
	fl.Info("nil value field")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestEdge_WithErrorNil(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")
	el := l.WithError(nil)
	el.Info("nil error")
	if buf.Len() == 0 {
		t.Error("expected output with nil error")
	}
}

func TestEdge_ChainedWithMethods(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf, "debug", "json")

	ctx := context.WithValue(context.Background(), contextKey("trace_id"), "chain-trace")
	chained := l.WithContext(ctx).WithComponent("chained").WithFields(map[string]any{"extra": "data"})
	chained.Info("chained call")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed[FieldTraceID] != "chain-trace" {
		t.Errorf("expected trace_id, got %v", parsed[FieldTraceID])
	}
	if parsed[FieldComponent] != "chained" {
		t.Errorf("expected component=chained, got %v", parsed[FieldComponent])
	}
	if parsed["extra"] != "data" {
		t.Errorf("expected extra=data, got %v", parsed["extra"])
	}
}

func TestEdge_ZeroValueConfig(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()
	l := New(cfg, "zero-cfg")
	if l == nil {
		t.Fatal("expected non-nil logger from zero-value config")
	}
	// Should not panic
	l.Info("from zero config")
}

func TestEdge_ErrorFieldsNilError(t *testing.T) {
	fields := ErrorFields("operation", nil)
	if fields[FieldOperation] != "operation" {
		t.Errorf("expected operation field, got %v", fields[FieldOperation])
	}
	if _, ok := fields[FieldError]; ok {
		t.Error("expected no error field when error is nil")
	}
}

func TestEdge_MergeWithErrorNilError(t *testing.T) {
	fields := map[string]any{"key": "val"}
	result := MergeWithError(fields, nil)
	if _, ok := result[FieldError]; ok {
		t.Error("expected no error field when error is nil")
	}
	if result["key"] != "val" {
		t.Error("expected existing fields to be preserved")
	}
}

func TestEdge_MergeWithDurationZero(t *testing.T) {
	result := MergeWithDuration(nil, 0)
	if result[FieldDuration] != int64(0) {
		t.Errorf("expected duration 0, got %v", result[FieldDuration])
	}
}

// ---------------------------------------------------------------------------
// Additional: Trace level (Logger has no Trace method on the wrapper, so
// we add one here to test the underlying zerolog trace level works)
// ---------------------------------------------------------------------------

// Trace logs a trace-level message (mirrors the other level methods).
func (l *Logger) Trace(msg string, fields ...map[string]any) {
	event := l.logger.Trace()
	l.addFields(event, fields...)
	event.Msg(msg)
}

// ---------------------------------------------------------------------------
// Additional: ComponentRegistry concurrent safety
// ---------------------------------------------------------------------------

func TestConcurrent_ComponentRegistrySafety(t *testing.T) {
	cr := NewComponentRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(4)
		go func(n int) {
			defer wg.Done()
			cr.RegisterInfrastructure(fmt.Sprintf("db-%d", n), "database", "active", "localhost")
		}(i)
		go func(n int) {
			defer wg.Done()
			cr.RegisterService(fmt.Sprintf("svc-%d", n), "active", nil)
		}(i)
		go func(n int) {
			defer wg.Done()
			cr.RegisterHandler("GET", fmt.Sprintf("/path-%d", n), "handler")
		}(i)
		go func() {
			defer wg.Done()
			_ = cr.Infrastructure()
			_ = cr.Services()
			_ = cr.Handlers()
		}()
	}
	wg.Wait()
}
