package logging

import (
	"errors"
	"testing"
	"time"
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
}

func TestConfigApplyDefaults(t *testing.T) {
	t.Parallel()

	c := Config{}
	c.ApplyDefaults()
	if c.Level != "info" || c.Format != "console" || c.Output != "stdout" {
		t.Errorf("core defaults not applied: %+v", c)
	}
	if !c.Timestamp {
		t.Error("timestamp should default to true")
	}
	if c.OTLP.Protocol != "grpc" || c.OTLP.Endpoint != "localhost:4317" {
		t.Errorf("OTLP defaults not applied: %+v", c.OTLP)
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
