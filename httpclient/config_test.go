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

func TestConfig_Validate_ZeroTimeout_AfterDefaults(t *testing.T) {
	cfg := Config{Timeout: 0}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected zero timeout to be defaulted, got error: %v", err)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", cfg.Timeout)
	}
}

func TestConfig_Validate_NegativeTimeout(t *testing.T) {
	cfg := Config{Timeout: -1 * time.Second}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Error("ApplyDefaults should have corrected negative timeout")
	}
}

func TestDefaultRetryConfig_HasRetryIf(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.RetryIf == nil {
		t.Fatal("expected RetryIf to be set")
	}
	// Should return true for retryable errors
	if !cfg.RetryIf(NewServerError(500, nil)) {
		t.Error("RetryIf should return true for server error")
	}
	if cfg.RetryIf(NewNotFoundError(nil)) {
		t.Error("RetryIf should return false for not-found error")
	}
}

func TestDefaultCircuitBreakerConfig_NotNil(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig("test-cb")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestDefaultRateLimiterConfig_NotNil(t *testing.T) {
	cfg := DefaultRateLimiterConfig("test-rl")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
