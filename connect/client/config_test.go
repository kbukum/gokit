package client

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/security"
)

func TestConfigApplyDefaults(t *testing.T) {
	var cfg Config
	cfg.ApplyDefaults()

	if cfg.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", cfg.Timeout, defaultTimeout)
	}
	if cfg.DialTimeout != defaultDialTimeout {
		t.Fatalf("DialTimeout = %s, want %s", cfg.DialTimeout, defaultDialTimeout)
	}
	if cfg.Protocol != ProtocolConnect {
		t.Fatalf("Protocol = %q, want %q", cfg.Protocol, ProtocolConnect)
	}
}

func TestConfigApplyDefaultsPreservesConfiguredValues(t *testing.T) {
	cfg := Config{
		BaseURL:     "https://example.test",
		Timeout:     time.Second,
		DialTimeout: 2 * time.Second,
		Protocol:    ProtocolGRPC,
	}
	cfg.ApplyDefaults()

	if cfg.BaseURL != "https://example.test" {
		t.Fatalf("BaseURL = %q, want configured URL", cfg.BaseURL)
	}
	if cfg.Timeout != time.Second {
		t.Fatalf("Timeout = %s, want 1s", cfg.Timeout)
	}
	if cfg.DialTimeout != 2*time.Second {
		t.Fatalf("DialTimeout = %s, want 2s", cfg.DialTimeout)
	}
	if cfg.Protocol != ProtocolGRPC {
		t.Fatalf("Protocol = %q, want grpc", cfg.Protocol)
	}
}

func TestConfigValidateProtocols(t *testing.T) {
	for _, protocol := range []string{ProtocolConnect, ProtocolGRPC, ProtocolGRPCWeb} {
		t.Run(protocol, func(t *testing.T) {
			cfg := Config{Protocol: protocol}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate returned error: %v", err)
			}
		})
	}
}

func TestConfigValidateRejectsUnsupportedProtocol(t *testing.T) {
	cfg := Config{Protocol: "unknown"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("error %q does not explain protocol failure", err.Error())
	}
}

func TestConfigValidatePropagatesTLSError(t *testing.T) {
	cfg := Config{Protocol: ProtocolConnect, TLS: &security.TLSConfig{MinVersion: tls.VersionTLS10}}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected TLS validation error")
	}
	if !strings.Contains(err.Error(), "TLS 1.2") {
		t.Fatalf("error %q does not mention TLS version", err.Error())
	}
}

func TestNoTimeoutKeepsTheClientUnbounded(t *testing.T) {
	cfg := Config{NoTimeout: true}
	cfg.ApplyDefaults()
	if cfg.Timeout != 0 {
		t.Fatalf("Timeout = %s, want 0 with NoTimeout", cfg.Timeout)
	}
	hc, err := NewHTTPClient(Config{NoTimeout: true})
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	if hc.Timeout != 0 {
		t.Fatalf("http.Client.Timeout = %s, want 0", hc.Timeout)
	}
}

func TestValidateRejectsNegativeTimeouts(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg     Config
		want    string
		without string
	}{
		"timeout":                 {cfg: Config{Timeout: -1}, want: "timeout must not be negative; use no_timeout"},
		"timeout with no_timeout": {cfg: Config{Timeout: -1, NoTimeout: true}, want: "timeout must not be negative"},
		"dial timeout":            {cfg: Config{DialTimeout: -1}, want: "dial_timeout must not be negative", without: "no_timeout"},
	} {
		tc.cfg.ApplyDefaults()
		err := tc.cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) || (tc.without != "" && strings.Contains(err.Error(), tc.without)) {
			t.Errorf("%s: Validate = %v, want %q without %q", name, err, tc.want, tc.without)
		}
	}
}

func TestValidateRejectsTimeoutWithNoTimeout(t *testing.T) {
	cfg := Config{Protocol: ProtocolConnect, NoTimeout: true, Timeout: time.Second}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "no_timeout") {
		t.Fatalf("Validate = %v, want a no_timeout conflict", err)
	}
}
