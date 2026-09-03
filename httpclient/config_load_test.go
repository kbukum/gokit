package httpclient

import (
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/kbukum/gokit/resilience"
)

// decodeConfig mirrors how gokit's config loader materializes a struct: a
// mapstructure decode with the string-to-duration hook, keyed by the shared
// snake_case names.
func decodeConfig(t *testing.T, raw map[string]any) Config {
	t.Helper()
	var cfg Config
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.TextUnmarshallerHookFunc(),
		),
		Result: &cfg,
	})
	if err != nil {
		t.Fatalf("new decoder: %v", err)
	}
	if err := dec.Decode(raw); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	return cfg
}

func TestConfig_LoadsSharedKeys(t *testing.T) {
	t.Parallel()

	cfg := decodeConfig(t, map[string]any{
		"name":                    "orders",
		"base_url":                "https://api.example.com",
		"timeout":                 "10s",
		"default_headers":         map[string]any{"X-Api-Key": "secret"},
		"max_response_body_bytes": 2048,
	})

	if cfg.Timeout != 10*time.Second {
		t.Errorf("timeout: got %s want 10s", cfg.Timeout)
	}
	if cfg.DefaultHeaders["X-Api-Key"] != "secret" {
		t.Errorf("default_headers: got %v", cfg.DefaultHeaders)
	}
	if cfg.MaxResponseBodyBytes != 2048 {
		t.Errorf("max_response_body_bytes: got %d want 2048", cfg.MaxResponseBodyBytes)
	}
}

func TestConfig_LoadsResiliencePolicy(t *testing.T) {
	t.Parallel()

	cfg := decodeConfig(t, map[string]any{
		"resilience_policy": map[string]any{
			"timeout": "3s",
			"retry": map[string]any{
				"max_attempts":    4,
				"initial_backoff": "50ms",
				"strategy":        "linear",
			},
			"circuit_breaker": map[string]any{
				"name":         "orders",
				"max_failures": 5,
			},
			"rate_limiter": map[string]any{
				"rate":  20.0,
				"burst": 10,
			},
		},
	})

	p := cfg.ResiliencePolicy
	if p == nil {
		t.Fatal("resilience_policy did not load")
	}
	if p.Timeout != 3*time.Second {
		t.Errorf("policy timeout: got %s want 3s", p.Timeout)
	}
	if p.Retry == nil || p.Retry.MaxAttempts != 4 || p.Retry.InitialBackoff != 50*time.Millisecond {
		t.Errorf("retry not loaded: %+v", p.Retry)
	}
	if p.Retry != nil && p.Retry.Strategy != resilience.LinearBackoff {
		t.Errorf("retry strategy: got %v want linear", p.Retry.Strategy)
	}
	if p.CircuitBreaker == nil || p.CircuitBreaker.MaxFailures != 5 {
		t.Errorf("circuit_breaker not loaded: %+v", p.CircuitBreaker)
	}
	if p.RateLimiter == nil || p.RateLimiter.Rate != 20.0 || p.RateLimiter.Burst != 10 {
		t.Errorf("rate_limiter not loaded: %+v", p.RateLimiter)
	}
}

func TestNew_ConfigLoadedRetryDefaultsToIsRetryable(t *testing.T) {
	t.Parallel()

	cfg := decodeConfig(t, map[string]any{
		"base_url": "https://api.example.com",
		"resilience_policy": map[string]any{
			"retry": map[string]any{"max_attempts": 3},
		},
	})

	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := adapter.GetConfig().ResiliencePolicy
	if got.Retry.RetryIf == nil {
		t.Fatal("config-loaded retry should default RetryIf to the HTTP-aware predicate")
	}
	// The HTTP-aware predicate retries a 5xx but not a 404, unlike the generic
	// default which retries every non-context error.
	if !got.Retry.RetryIf(NewServerError(500, nil)) {
		t.Error("expected server error to be retryable")
	}
	if got.Retry.RetryIf(NewNotFoundError(nil)) {
		t.Error("expected not-found error to be non-retryable")
	}
}

// TestNew_RetryDefaultDoesNotMutateSourcePolicy locks in the Clone race fix: New
// must default a nil RetryIf on a copy, never on the caller-owned policy that may
// be shared across concurrently constructed adapters.
func TestNew_RetryDefaultDoesNotMutateSourcePolicy(t *testing.T) {
	t.Parallel()

	source := resilience.NewPolicy().WithRetry(resilience.RetryConfig{MaxAttempts: 3})
	cfg := Config{BaseURL: "https://api.example.com", ResiliencePolicy: source}

	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if source.Retry.RetryIf != nil {
		t.Error("New must not default RetryIf on the caller-owned source policy")
	}

	got := adapter.GetConfig().ResiliencePolicy
	if got == source {
		t.Error("adapter must own a distinct policy pointer, not the source policy")
	}
	if got.Retry == source.Retry {
		t.Error("adapter must own a distinct retry pointer, not the source retry block")
	}
	if got.Retry.RetryIf == nil {
		t.Error("adapter's cloned retry should default RetryIf to the HTTP-aware predicate")
	}
}
