package security

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/security/tlstest"
)

func TestTLSConfig_Handshake_PrefersModernTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certs.ServerTLS},
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	defer server.Close()

	tlsCfg, err := (&TLSConfig{CAFile: certs.CAFile, ServerName: "localhost"}).Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("GET() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.TLS == nil {
		t.Fatal("expected TLS connection state")
	}
	if resp.TLS.Version != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3 negotiation, got %#x", resp.TLS.Version)
	}
}

func TestTLSConfig_Handshake_RejectsObsoleteTLS(t *testing.T) {
	certs := tlstest.GenerateTLSCerts(t)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{certs.ServerTLS},
		MinVersion:   tls.VersionTLS12,
	}
	server.StartTLS()
	defer server.Close()

	tlsCfg, err := (&TLSConfig{CAFile: certs.CAFile, ServerName: "localhost"}).Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	tlsCfg.MaxVersion = tls.VersionTLS11

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	resp, err := client.Get(server.URL)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected handshake failure for TLS 1.1 client")
	}
}
