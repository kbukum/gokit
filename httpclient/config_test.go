package httpclient

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/security"
)

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", cfg.Timeout)
	}
}

func TestConfig_ApplyDefaults_PreservesExisting(t *testing.T) {
	cfg := Config{Timeout: 10 * time.Second}
	cfg.ApplyDefaults()
	if cfg.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", cfg.Timeout)
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{Timeout: 10 * time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_InvalidTimeout(t *testing.T) {
	cfg := Config{Timeout: -1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestConfig_Validate_InvalidTLS(t *testing.T) {
	cfg := Config{
		Timeout: 10 * time.Second,
		TLS:     &security.TLSConfig{CertFile: "cert.pem"}, // missing KeyFile
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for mismatched TLS cert/key")
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.MaxAttempts <= 0 {
		t.Error("expected positive MaxAttempts")
	}
	if cfg.RetryIf == nil {
		t.Error("expected RetryIf to be set")
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig("test")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig("test")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}
}
