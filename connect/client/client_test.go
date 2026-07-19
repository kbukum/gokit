package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/kbukum/gokit/security"
)

func TestNewHTTPClientBuildsH2CClientByDefault(t *testing.T) {
	client, err := NewHTTPClient(Config{})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	if client.Timeout != defaultTimeout {
		t.Fatalf("Timeout = %s, want %s", client.Timeout, defaultTimeout)
	}
	transport, ok := client.Transport.(*http2.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http2.Transport", client.Transport)
	}
	if !transport.AllowHTTP {
		t.Fatal("default transport should allow h2c")
	}
	if transport.TLSClientConfig != nil {
		t.Fatal("default h2c transport should not configure TLS")
	}
}

func TestNewHTTPClientRejectsInvalidConfig(t *testing.T) {
	client, err := NewHTTPClient(Config{Protocol: "invalid"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if client != nil {
		t.Fatalf("client = %#v, want nil", client)
	}
	if !strings.Contains(err.Error(), "connect client") {
		t.Fatalf("error %q should include client context", err.Error())
	}
}

func TestNewHTTPClientPropagatesTLSBuildError(t *testing.T) {
	client, err := NewHTTPClient(Config{
		TLS: &security.TLSConfig{CAFile: "missing-ca.pem"},
	})
	if err == nil {
		t.Fatal("expected TLS build error")
	}
	if client != nil {
		t.Fatalf("client = %#v, want nil", client)
	}
	if !strings.Contains(err.Error(), "failed to read CA file") {
		t.Fatalf("error %q does not include TLS cause", err.Error())
	}
}

func TestNewHTTPClientBuildsTLSTransport(t *testing.T) {
	client, err := NewHTTPClient(Config{
		Timeout: 5 * time.Second,
		TLS:     &security.TLSConfig{SkipVerify: true, ServerName: "example.test"},
	})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %s, want 5s", client.Timeout)
	}
	transport, ok := client.Transport.(*http2.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http2.Transport", client.Transport)
	}
	if transport.AllowHTTP {
		t.Fatal("TLS transport should not allow cleartext HTTP")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLS transport should include TLS config")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS transport should carry SkipVerify setting")
	}
	if transport.TLSClientConfig.ServerName != "example.test" {
		t.Fatalf("ServerName = %q, want example.test", transport.TLSClientConfig.ServerName)
	}
}

func TestBuildH2CTransportDialUsesContext(t *testing.T) {
	transport := buildH2CTransport(Config{DialTimeout: time.Minute}).(*http2.Transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := transport.DialTLSContext(ctx, "tcp", "127.0.0.1:1", nil)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected canceled dial to fail")
	}
	if conn != nil {
		t.Fatalf("conn = %#v, want nil", conn)
	}
}

func TestBuildTLSTransportDialUsesContext(t *testing.T) {
	transport, err := buildTLSTransport(Config{
		DialTimeout: time.Minute,
		TLS:         &security.TLSConfig{SkipVerify: true},
	})
	if err != nil {
		t.Fatalf("buildTLSTransport returned error: %v", err)
	}
	h2Transport := transport.(*http2.Transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := h2Transport.DialTLSContext(ctx, "tcp", "127.0.0.1:1", &tls.Config{MinVersion: tls.VersionTLS12})
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected canceled dial to fail")
	}
	if conn != nil {
		t.Fatalf("conn = %#v, want nil", conn)
	}
}

func TestBuildTransportWithDisabledTLSUsesH2C(t *testing.T) {
	transport, err := buildTransport(Config{TLS: &security.TLSConfig{}})
	if err != nil {
		t.Fatalf("buildTransport returned error: %v", err)
	}
	h2Transport, ok := transport.(*http2.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http2.Transport", transport)
	}
	if !h2Transport.AllowHTTP {
		t.Fatal("disabled TLS config should still use h2c")
	}
}

func TestProtocolOption(t *testing.T) {
	if opt := ProtocolOption(Config{Protocol: ProtocolConnect}); opt != nil {
		t.Fatalf("connect protocol option = %#v, want nil", opt)
	}
	if opt := ProtocolOption(Config{}); opt != nil {
		t.Fatalf("zero-value protocol option = %#v, want nil", opt)
	}
	if opt := ProtocolOption(Config{Protocol: ProtocolGRPC}); opt == nil {
		t.Fatal("grpc protocol should return option")
	}
	if opt := ProtocolOption(Config{Protocol: ProtocolGRPCWeb}); opt == nil {
		t.Fatal("grpcweb protocol should return option")
	}
}

func TestClientOptions(t *testing.T) {
	if opts := ClientOptions(Config{Protocol: ProtocolConnect}); len(opts) != 0 {
		t.Fatalf("ClientOptions(connect) returned %d options, want 0", len(opts))
	}
	if opts := ClientOptions(Config{Protocol: ProtocolGRPC}); len(opts) != 1 {
		t.Fatalf("ClientOptions(grpc) returned %d options, want 1", len(opts))
	}
}

var (
	_ net.Conn
	_ http.RoundTripper
)
