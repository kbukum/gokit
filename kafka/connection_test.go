package kafka

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/security/tlstest"
)

func TestResolveCompression(t *testing.T) {
	tests := []struct {
		name     string
		expected kafkago.Compression
	}{
		{"gzip", kafkago.Gzip},
		{"lz4", kafkago.Lz4},
		{"zstd", kafkago.Zstd},
		{"snappy", kafkago.Snappy},
		{"none", 0},
		{"unknown", kafkago.Snappy},
		{"", kafkago.Snappy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveCompression(tt.name); got != tt.expected {
				t.Errorf("ResolveCompression(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestBuildSASLMechanism_PLAIN(t *testing.T) {
	cfg := &Config{SASLMechanism: "PLAIN", Username: "user", Password: "pass"}
	m, err := buildSASLMechanism(cfg)
	if err != nil {
		t.Fatalf("buildSASLMechanism() error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mechanism")
	}
}

func TestBuildSASLMechanism_SCRAM256(t *testing.T) {
	cfg := &Config{SASLMechanism: "SCRAM-SHA-256", Username: "user", Password: "pass"}
	m, err := buildSASLMechanism(cfg)
	if err != nil {
		t.Fatalf("buildSASLMechanism() error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mechanism")
	}
}

func TestBuildSASLMechanism_SCRAM512(t *testing.T) {
	cfg := &Config{SASLMechanism: "SCRAM-SHA-512", Username: "user", Password: "pass"}
	m, err := buildSASLMechanism(cfg)
	if err != nil {
		t.Fatalf("buildSASLMechanism() error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil mechanism")
	}
}

func TestBuildSASLMechanism_Unsupported(t *testing.T) {
	cfg := &Config{SASLMechanism: "KERBEROS"}
	_, err := buildSASLMechanism(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported mechanism")
	}
}

func TestCreateTransport_NoTLS_NoSASL(t *testing.T) {
	cfg := &Config{IdleTimeout: "30s", MetadataTTL: "6s"}
	transport, err := CreateTransport(cfg)
	if err != nil {
		t.Fatalf("CreateTransport() error: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport.TLS != nil {
		t.Error("expected nil TLS for non-TLS config")
	}
}

func TestCreateTransport_WithTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &Config{
		IdleTimeout: "30s",
		MetadataTTL: "6s",
		TLS:         &security.TLSConfig{CAFile: certs.CAFile},
	}
	transport, err := CreateTransport(cfg)
	if err != nil {
		t.Fatalf("CreateTransport() error: %v", err)
	}
	if transport.TLS == nil {
		t.Error("expected non-nil TLS config")
	}
}

func TestCreateTransport_InvalidTLS(t *testing.T) {
	cfg := &Config{
		TLS: &security.TLSConfig{CAFile: "/nonexistent/ca.pem"},
	}
	_, err := CreateTransport(cfg)
	if err == nil {
		t.Fatal("expected error for invalid TLS config")
	}
}

func TestCreateTransport_WithSASL(t *testing.T) {
	cfg := &Config{
		IdleTimeout:   "30s",
		MetadataTTL:   "6s",
		EnableSASL:    true,
		SASLMechanism: "PLAIN",
		Username:      "user",
		Password:      "pass",
	}
	transport, err := CreateTransport(cfg)
	if err != nil {
		t.Fatalf("CreateTransport() error: %v", err)
	}
	if transport.SASL == nil {
		t.Error("expected non-nil SASL mechanism")
	}
}

func TestCreateTransport_InvalidSASL(t *testing.T) {
	cfg := &Config{
		EnableSASL:    true,
		SASLMechanism: "INVALID",
	}
	_, err := CreateTransport(cfg)
	if err == nil {
		t.Fatal("expected error for invalid SASL mechanism")
	}
}

func TestCreateDialer_NoTLS_NoSASL(t *testing.T) {
	cfg := &Config{DialTimeout: "10s"}
	dialer, err := CreateDialer(cfg)
	if err != nil {
		t.Fatalf("CreateDialer() error: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil dialer")
	}
	if !dialer.DualStack {
		t.Error("expected DualStack=true")
	}
	if dialer.TLS != nil {
		t.Error("expected nil TLS")
	}
}

func TestCreateDialer_WithTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &Config{
		DialTimeout: "10s",
		TLS:         &security.TLSConfig{CAFile: certs.CAFile},
	}
	dialer, err := CreateDialer(cfg)
	if err != nil {
		t.Fatalf("CreateDialer() error: %v", err)
	}
	if dialer.TLS == nil {
		t.Error("expected non-nil TLS config")
	}
}

func TestCreateDialer_InvalidTLS(t *testing.T) {
	cfg := &Config{
		TLS: &security.TLSConfig{CAFile: "/nonexistent/ca.pem"},
	}
	_, err := CreateDialer(cfg)
	if err == nil {
		t.Fatal("expected error for invalid TLS config")
	}
}

func TestCreateDialer_WithSASL(t *testing.T) {
	cfg := &Config{
		DialTimeout:   "10s",
		EnableSASL:    true,
		SASLMechanism: "SCRAM-SHA-256",
		Username:      "user",
		Password:      "pass",
	}
	dialer, err := CreateDialer(cfg)
	if err != nil {
		t.Fatalf("CreateDialer() error: %v", err)
	}
	if dialer.SASLMechanism == nil {
		t.Error("expected non-nil SASL mechanism")
	}
}

func TestCreateDialer_InvalidSASL(t *testing.T) {
	cfg := &Config{
		EnableSASL:    true,
		SASLMechanism: "INVALID",
	}
	_, err := CreateDialer(cfg)
	if err == nil {
		t.Fatal("expected error for invalid SASL mechanism")
	}
}
