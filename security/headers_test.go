package security

import (
	"net/http"
	"strconv"
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

// The default Permissions-Policy is the eight-directive shared cross-kit policy
// (accelerometer, camera, geolocation, gyroscope, magnetometer, microphone,
// payment, usb), each locked to the empty allowlist.
func TestHeadersConfig_PermissionsPolicyEightDirectives(t *testing.T) {
	t.Parallel()

	var cfg HeadersConfig
	headers, err := cfg.HeaderMap()
	if err != nil {
		t.Fatalf("HeaderMap: %v", err)
	}
	got := headers["Permissions-Policy"]
	directives := strings.Split(got, ", ")
	if len(directives) != 8 {
		t.Fatalf("expected 8 Permissions-Policy directives, got %d: %q", len(directives), got)
	}
	want := []string{
		"accelerometer=()", "camera=()", "geolocation=()", "gyroscope=()",
		"magnetometer=()", "microphone=()", "payment=()", "usb=()",
	}
	for i, w := range want {
		if directives[i] != w {
			t.Fatalf("directive %d = %q, want %q (full: %q)", i, directives[i], w, got)
		}
	}
}

// HSTS max-age is caller-configurable and reflected verbatim (in seconds) in the
// Strict-Transport-Security header.
func TestHeadersConfig_HSTSMaxAgeConfigurable(t *testing.T) {
	t.Parallel()

	cfg := HeadersConfig{HSTSMaxAge: 90 * 24 * time.Hour}
	headers, err := cfg.HeaderMap()
	if err != nil {
		t.Fatalf("HeaderMap: %v", err)
	}
	const wantSeconds = int64(90 * 24 * 60 * 60)
	want := "max-age=" + strconv.FormatInt(wantSeconds, 10)
	if !strings.HasPrefix(headers["Strict-Transport-Security"], want) {
		t.Fatalf("HSTS = %q, want prefix %q", headers["Strict-Transport-Security"], want)
	}
}
