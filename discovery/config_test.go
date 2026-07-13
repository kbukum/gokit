package discovery

import (
	"errors"
	"testing"
	"time"
)

// ── Config validation ───────────────────────────────────────────────

func TestConfig_Validate_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled config should pass validation, got: %v", err)
	}
}

func TestConfig_Validate_MissingServiceName(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Registration: RegistrationConfig{ServicePort: 8080},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing service_name")
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Registration: RegistrationConfig{
			ServiceName: "test",
			ServicePort: 0,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for zero port")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Registration: RegistrationConfig{
			ServiceName: "test",
			ServicePort: 8080,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	if cfg.Provider != "static" {
		t.Errorf("default provider = %q, want %q", cfg.Provider, "static")
	}
	if cfg.Health.Type != HealthCheckHTTP {
		t.Errorf("default health type = %q, want %q", cfg.Health.Type, HealthCheckHTTP)
	}
	if cfg.Health.Path != "/healthz" {
		t.Errorf("default health path = %q, want %q", cfg.Health.Path, "/healthz")
	}
	if cfg.Health.Interval != "10s" {
		t.Errorf("default interval = %q, want %q", cfg.Health.Interval, "10s")
	}
}

func TestConfig_ApplyDefaults_RegistrationServiceID(t *testing.T) {
	cfg := Config{
		Registration: RegistrationConfig{ServiceName: "my-svc"},
	}
	cfg.ApplyDefaults()
	if cfg.Registration.ServiceID != "my-svc" {
		t.Errorf("ServiceID = %q, want %q (should default to ServiceName)", cfg.Registration.ServiceID, "my-svc")
	}
}

func TestConfig_BuildClientConfig(t *testing.T) {
	cfg := Config{
		CacheTTL: "15s",
		Services: []DiscoveredService{
			{Name: "api", Criticality: CriticalityRequired},
			{Name: "web", Criticality: CriticalityOptional},
		},
	}
	cc := cfg.BuildClientConfig()
	if cc.CacheTTL != 15*time.Second {
		t.Errorf("CacheTTL = %v, want %v", cc.CacheTTL, 15*time.Second)
	}
	if len(cc.Services) != 2 {
		t.Errorf("Services count = %d, want 2", len(cc.Services))
	}
	if cc.Criticality["api"] != CriticalityRequired {
		t.Errorf("api criticality = %q, want %q", cc.Criticality["api"], CriticalityRequired)
	}
}

func TestParseDuration_Empty(t *testing.T) {
	d := ParseDuration("")
	if d != 0 {
		t.Errorf("ParseDuration('') = %v, want 0", d)
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	d := ParseDuration("not-a-duration")
	if d != 0 {
		t.Errorf("ParseDuration('not-a-duration') = %v, want 0", d)
	}
}

func TestConfigRetryConfigAndResolveAddr(t *testing.T) {
	t.Parallel()

	retry := RegistrationConfig{MaxRetries: 4, RetryInterval: "250ms"}.RetryConfig()
	if retry.MaxAttempts != 4 || retry.InitialBackoff != 250*time.Millisecond || retry.MaxBackoff != 2*time.Second {
		t.Fatalf("RetryConfig = %+v, want attempts=4 initial=250ms max=2s", retry)
	}
	fallbackRetry := RegistrationConfig{MaxRetries: 2, RetryInterval: "bad"}.RetryConfig()
	if fallbackRetry.InitialBackoff != 2*time.Second {
		t.Fatalf("RetryConfig invalid interval initial = %v, want 2s", fallbackRetry.InitialBackoff)
	}

	mock := &mockDiscovery{instances: map[string][]ServiceInstance{"db": {makeInstance("db-1", "db", "127.0.0.1", 5432)}}}
	host, port, err := ResolveAddr(mock, "db")
	if err != nil {
		t.Fatalf("ResolveAddr: %v", err)
	}
	if host != "127.0.0.1" || port != 5432 {
		t.Fatalf("ResolveAddr = %s:%d, want 127.0.0.1:5432", host, port)
	}
	mock.err = errors.New("backend down")
	if _, _, err := ResolveAddr(mock, "db"); err == nil {
		t.Fatal("ResolveAddr backend error succeeded, want error")
	}
	mock.err = nil
	if _, _, err := ResolveAddr(mock, "missing"); err == nil {
		t.Fatal("ResolveAddr missing service succeeded, want error")
	}
}
