package security

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHeadersConfig_Defaults(t *testing.T) {
	t.Parallel()

	var cfg HeadersConfig
	headers, err := cfg.HeaderMap()
	if err != nil {
		t.Fatalf("HeaderMap: %v", err)
	}

	if headers["X-Content-Type-Options"] != "nosniff" {
		t.Fatalf("expected nosniff, got %q", headers["X-Content-Type-Options"])
	}
	if headers["X-Frame-Options"] != "DENY" {
		t.Fatalf("expected DENY, got %q", headers["X-Frame-Options"])
	}
	if !strings.Contains(headers["Strict-Transport-Security"], "includeSubDomains") {
		t.Fatalf("expected includeSubDomains in HSTS, got %q", headers["Strict-Transport-Security"])
	}
	if !strings.Contains(headers["Content-Security-Policy"], "frame-ancestors 'none'") {
		t.Fatalf("expected secure CSP, got %q", headers["Content-Security-Policy"])
	}
}

func TestHeadersConfig_Customization(t *testing.T) {
	t.Parallel()

	cfg := HeadersConfig{
		HSTSMaxAge:                   24 * time.Hour,
		DisableHSTSIncludeSubdomains: true,
		DisableHSTSPreload:           true,
		XFrameOptions:                "SAMEORIGIN",
	}
	headers, err := cfg.HeaderMap()
	if err != nil {
		t.Fatalf("HeaderMap: %v", err)
	}

	if headers["X-Frame-Options"] != "SAMEORIGIN" {
		t.Fatalf("unexpected x-frame-options: %q", headers["X-Frame-Options"])
	}
	if strings.Contains(headers["Strict-Transport-Security"], "includeSubDomains") {
		t.Fatalf("did not expect includeSubDomains in HSTS: %q", headers["Strict-Transport-Security"])
	}
}

func TestHeadersConfig_Apply(t *testing.T) {
	t.Parallel()

	var cfg HeadersConfig
	header := http.Header{}
	if err := cfg.Apply(header); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if header.Get("Referrer-Policy") == "" {
		t.Fatal("expected Referrer-Policy to be set")
	}
}

func TestHeadersConfig_InvalidFrameOptionsRejected(t *testing.T) {
	t.Parallel()

	cfg := HeadersConfig{
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		XFrameOptions:         "ALLOWALL",
	}
	if _, err := cfg.HeaderMap(); err == nil {
		t.Fatal("expected invalid x-frame-options to fail")
	}
}

func TestHeadersConfig_Disabled(t *testing.T) {
	t.Parallel()

	cfg := HeadersConfig{Disabled: true}
	headers, err := cfg.HeaderMap()
	if err != nil {
		t.Fatalf("HeaderMap: %v", err)
	}
	if len(headers) != 0 {
		t.Fatalf("expected no headers when disabled, got %+v", headers)
	}
}
