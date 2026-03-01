package security

import (
	"crypto/tls"
	"testing"

	"github.com/kbukum/gokit/security/tlstest"
)

func TestTLSConfig_Build_NilConfig(t *testing.T) {
	var cfg *TLSConfig
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil for nil config")
	}
}

func TestTLSConfig_Build_ZeroValue(t *testing.T) {
	cfg := &TLSConfig{}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil for zero-value config")
	}
}

func TestTLSConfig_Build_SkipVerify(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if !result.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion=TLS12, got %d", result.MinVersion)
	}
}

func TestTLSConfig_Build_ServerName(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true, ServerName: "example.com"}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ServerName != "example.com" {
		t.Errorf("expected ServerName=example.com, got %s", result.ServerName)
	}
}

func TestTLSConfig_Build_CustomMinVersion(t *testing.T) {
	cfg := &TLSConfig{SkipVerify: true, MinVersion: tls.VersionTLS13}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected MinVersion=TLS13, got %d", result.MinVersion)
	}
}

func TestTLSConfig_Build_InvalidCAFile(t *testing.T) {
	cfg := &TLSConfig{CAFile: "/nonexistent/ca.pem"}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for nonexistent CA file")
	}
}

func TestTLSConfig_Build_InvalidCertFiles(t *testing.T) {
	cfg := &TLSConfig{CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for nonexistent cert files")
	}
}

func TestTLSConfig_Validate_Nil(t *testing.T) {
	var cfg *TLSConfig
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTLSConfig_Validate_Valid(t *testing.T) {
	cfg := &TLSConfig{CertFile: "cert.pem", KeyFile: "key.pem"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTLSConfig_Validate_MismatchedCertKey(t *testing.T) {
	cfg := &TLSConfig{CertFile: "cert.pem"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when CertFile set without KeyFile")
	}

	cfg = &TLSConfig{KeyFile: "key.pem"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when KeyFile set without CertFile")
	}
}

func TestTLSConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *TLSConfig
		enabled bool
	}{
		{"nil", nil, false},
		{"zero", &TLSConfig{}, false},
		{"skip_verify", &TLSConfig{SkipVerify: true}, true},
		{"ca_file", &TLSConfig{CAFile: "ca.pem"}, true},
		{"cert_file", &TLSConfig{CertFile: "cert.pem"}, true},
		{"server_name", &TLSConfig{ServerName: "example.com"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsEnabled(); got != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.enabled)
			}
		})
	}
}

func TestTLSConfig_Build_ValidCA(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{CAFile: certs.CAFile}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if result.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
}

func TestTLSConfig_Build_ValidClientCert(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CertFile: certs.CertFile,
		KeyFile:  certs.KeyFile,
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if len(result.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(result.Certificates))
	}
}

func TestTLSConfig_Build_FullConfig(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)
	cfg := &TLSConfig{
		CAFile:     certs.CAFile,
		CertFile:   certs.CertFile,
		KeyFile:    certs.KeyFile,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
	}
	result, err := cfg.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if result.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
	if len(result.Certificates) != 1 {
		t.Error("expected 1 client certificate")
	}
	if result.ServerName != "localhost" {
		t.Errorf("expected ServerName=localhost, got %s", result.ServerName)
	}
	if result.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected MinVersion=TLS13, got %d", result.MinVersion)
	}
}

func TestTLSConfig_Build_InvalidCAContent(t *testing.T) {
	caFile := tlstest.WriteInvalidPEM(t, "bad-ca.pem")
	cfg := &TLSConfig{CAFile: caFile}
	_, err := cfg.Build()
	if err == nil {
		t.Fatal("expected error for invalid CA PEM content")
	}
}
