package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

// NewHTTPClient creates an *http.Client configured for ConnectRPC.
//
// When TLS is configured, a standard HTTPS transport is used.
// When TLS is nil (the default), an h2c (cleartext HTTP/2) transport is used,
// which is required for ConnectRPC and gRPC communication without TLS.
//
// The returned client can be passed directly to any generated Connect client constructor.
func NewHTTPClient(cfg Config) (*http.Client, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}

	transport, err := buildTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect client: %w", err)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}, nil
}

// buildTransport creates the appropriate HTTP transport based on TLS config.
func buildTransport(cfg Config) (http.RoundTripper, error) {
	if cfg.TLS != nil && cfg.TLS.IsEnabled() {
		return buildTLSTransport(cfg)
	}
	return buildH2CTransport(cfg), nil
}

// buildH2CTransport creates an h2c (cleartext HTTP/2) transport.
func buildH2CTransport(cfg Config) http.RoundTripper {
	dialTimeout := cfg.DialTimeout
	return &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, addr)
		},
	}
}

// buildTLSTransport creates a standard HTTPS transport with the configured TLS settings.
func buildTLSTransport(cfg Config) (http.RoundTripper, error) {
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return nil, err
	}

	return &http2.Transport{
		TLSClientConfig: tlsCfg,
		DialTLSContext: func(ctx context.Context, network, addr string, tlsConf *tls.Config) (net.Conn, error) {
			dialer := &tls.Dialer{
				NetDialer: &net.Dialer{Timeout: cfg.DialTimeout},
				Config:    tlsConf,
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}, nil
}

// ProtocolOption returns the connect.ClientOption for the configured wire protocol.
// Returns nil for the default Connect protocol (no option needed).
func ProtocolOption(cfg Config) connect.ClientOption {
	switch cfg.Protocol {
	case ProtocolGRPC:
		return connect.WithGRPC()
	case ProtocolGRPCWeb:
		return connect.WithGRPCWeb()
	default:
		return nil
	}
}

// ClientOptions returns connect.ClientOption slice based on config.
// Includes protocol option if non-default protocol is configured.
func ClientOptions(cfg Config) []connect.ClientOption {
	var opts []connect.ClientOption
	if opt := ProtocolOption(cfg); opt != nil {
		opts = append(opts, opt)
	}
	return opts
}
