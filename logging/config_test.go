package logging

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/kbukum/gokit/contracttest/golden"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	if err := (&Config{Level: "info", Format: "json"}).Validate(); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
	if err := (&Config{Level: "bogus", Format: "json"}).Validate(); err == nil {
		t.Error("invalid level should be rejected")
	}
	if err := (&Config{Level: "info", Format: "bogus"}).Validate(); err == nil {
		t.Error("invalid format should be rejected")
	}
	for _, format := range []string{"json", "console", "text", FormatPretty} {
		if err := (&Config{Level: "info", Format: format}).Validate(); err != nil {
			t.Errorf("format %q should be accepted: %v", format, err)
		}
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	t.Parallel()

	c := Config{}
	c.ApplyDefaults()
	if c.Level != "info" || c.Format != "console" || c.Output != OutputStdout() {
		t.Errorf("core defaults not applied: %+v", c)
	}
	if !c.Timestamp {
		t.Error("timestamp should default to true")
	}
	if c.OTLP.Protocol != "grpc" || c.OTLP.Endpoint != "localhost:4317" {
		t.Errorf("OTLP defaults not applied: %+v", c.OTLP)
	}
}

func TestConfigApplyDefaultsUsesSharedRedactionToken(t *testing.T) {
	t.Parallel()

	c := Config{}
	c.ApplyDefaults()
	if c.Masking.Replacement != "[REDACTED]" {
		t.Errorf("Masking.Replacement = %q, want [REDACTED]", c.Masking.Replacement)
	}
}

func TestConfigOutputIsTypedWireEnum(t *testing.T) {
	t.Parallel()

	outputField, ok := reflect.TypeOf(Config{}).FieldByName("Output")
	if !ok {
		t.Fatal("Config.Output field missing")
	}
	if outputField.Type != reflect.TypeOf(Output{}) {
		t.Fatalf("Config.Output type = %v, want logging.Output", outputField.Type)
	}
	if outputField.Tag.Get("yaml") != "output" || outputField.Tag.Get("mapstructure") != "output" {
		t.Fatalf("Config.Output tags = %q, want yaml/mapstructure output", outputField.Tag)
	}
	if err := (&Config{Level: "info", Format: "json", Output: OutputStderr()}).Validate(); err != nil {
		t.Fatalf("typed stderr output should validate: %v", err)
	}
	if err := (&Config{Level: "info", Format: "json", Output: Output{Type: OutputType("socket")}}).Validate(); err == nil {
		t.Fatal("invalid typed output should be rejected")
	}
}

func TestConfigOutputAcceptsStringAndTaggedFileForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  Output
	}{
		{"stdout string", "stdout", OutputStdout()},
		{"stderr string", "stderr", OutputStderr()},
		{"file tagged", map[string]any{"type": "file", "path": "/p.log"}, OutputFile("/p.log")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseOutput(tc.input)
			if err != nil {
				t.Fatalf("ParseOutput: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseOutput = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestConfigOutputTaggedFileGoldenJSON(t *testing.T) {
	t.Parallel()

	body, err := json.Marshal(struct {
		Output Output `json:"output"`
	}{Output: OutputFile("/p.log")})
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	golden.AssertJSON(t, body, `{"output":{"type":"file","path":"/p.log"}}`)
}

func TestConfigOutputFileRequiresPath(t *testing.T) {
	t.Parallel()

	if err := (&Config{Level: "info", Format: "json", Output: Output{Type: OutputTypeFile}}).Validate(); err == nil {
		t.Fatal("file output without path should be invalid")
	}
}

func TestConfigOutputMapstructureDecodeHook(t *testing.T) {
	t.Parallel()

	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &cfg,
		DecodeHook: OutputDecodeHook(),
	})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := decoder.Decode(map[string]any{"output": map[string]any{"type": "file", "path": "/p.log"}}); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Output != OutputFile("/p.log") {
		t.Fatalf("Output = %+v, want file output", cfg.Output)
	}
}

func TestConfigCallerWireKeyIsCaller(t *testing.T) {
	t.Parallel()

	callerField, ok := reflect.TypeOf(Config{}).FieldByName("Caller")
	if !ok {
		t.Fatal("Config.Caller field missing")
	}
	if callerField.Tag.Get("yaml") != "caller" || callerField.Tag.Get("mapstructure") != "caller" {
		t.Fatalf("Config.Caller tags = %q, want yaml/mapstructure caller", callerField.Tag)
	}

	var cfg Config
	if err := mapstructure.Decode(map[string]any{"caller": true, "with_caller": false}, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if !cfg.Caller {
		t.Fatal("caller is the loadable cross-kit key; with_caller must not replace it")
	}
}

func TestFieldHelpers(t *testing.T) {
	t.Parallel()

	if f := DurationFields("save", 1500*time.Millisecond); f[FieldOperation] != "save" || f[FieldDuration] != int64(1500) {
		t.Errorf("DurationFields = %v", f)
	}
	if f := MergeWithError(nil, errors.New("boom")); f[FieldError] == nil {
		t.Error("MergeWithError should add an error field")
	}
	if f := MergeWithDuration(nil, time.Second); f[FieldDuration] != int64(1000) {
		t.Errorf("MergeWithDuration = %v", f)
	}
	sf := ServiceFields("svc", "prod", "1.0.0")
	if sf[FieldService] != "svc" || sf[FieldEnvironment] != "prod" || sf[FieldVersion] != "1.0.0" {
		t.Errorf("ServiceFields = %v", sf)
	}
}
