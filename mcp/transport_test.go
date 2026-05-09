package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseTransport(t *testing.T) {
	t.Parallel()

	if transport, err := ParseTransport("stdio"); err != nil || transport != TransportStdio {
		t.Fatalf("ParseTransport(stdio) = %q, %v", transport, err)
	}
	if transport, err := ParseTransport("streamable_http"); err != nil || transport != TransportStreamableHTTP {
		t.Fatalf("ParseTransport(streamable_http) = %q, %v", transport, err)
	}
	if _, err := ParseTransport("sse"); err == nil {
		t.Fatal("ParseTransport(sse) should reject obsolete transport names")
	}
}

func TestNewStreamableHTTPOptions(t *testing.T) {
	t.Parallel()

	opts, err := NewStreamableHTTPOptions(StreamableHTTPConfig{
		AllowedOrigins: []string{"HTTPS://APP.EXAMPLE.COM"},
	})
	if err != nil {
		t.Fatalf("NewStreamableHTTPOptions() error = %v", err)
	}
	if opts.DisableLocalhostProtection {
		t.Fatal("localhost protection should stay enabled by default")
	}

	trusted := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", http.NoBody)
	trusted.Host = "127.0.0.1"
	trusted.Header.Set("Origin", "https://app.example.com")
	if err := opts.CrossOriginProtection.Check(trusted); err != nil {
		t.Fatalf("trusted origin rejected: %v", err)
	}

	untrusted := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", http.NoBody)
	untrusted.Host = "127.0.0.1"
	untrusted.Header.Set("Origin", "https://evil.example.com")
	if err := opts.CrossOriginProtection.Check(untrusted); err == nil {
		t.Fatal("untrusted origin should be rejected")
	}
}

func TestNewStreamableHTTPOptionsInvalidOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
	}{
		{name: "missing scheme", origin: "localhost:3000"},
		{name: "path", origin: "https://app.example.com/mcp"},
		{name: "query", origin: "https://app.example.com?foo=bar"},
		{name: "fragment", origin: "https://app.example.com#fragment"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewStreamableHTTPOptions(StreamableHTTPConfig{
				AllowedOrigins: []string{tc.origin},
			}); err == nil {
				t.Fatalf("NewStreamableHTTPOptions() should reject %s origin %q", tc.name, tc.origin)
			}
		})
	}
}
