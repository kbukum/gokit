package client

import (
	"fmt"
	"net"
	"net/http"

	"connectrpc.com/connect"
)

// NewHTTPClient creates an *http.Client configured for ConnectRPC.
//
// When TLS is configured, an HTTPS-only HTTP/2 transport is used. When TLS is nil (the default),
// an h2c (cleartext HTTP/2) transport is used, which is required for ConnectRPC
// and gRPC communication without TLS.
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
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	return &schemeTransport{
		scheme: "http",
		Transport: &http.Transport{
			Protocols:   protocols,
			DialContext: (&net.Dialer{Timeout: cfg.DialTimeout}).DialContext,
		},
	}
}

// buildTLSTransport creates a standard HTTPS transport with the configured TLS settings.
func buildTLSTransport(cfg Config) (http.RoundTripper, error) {
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return nil, err
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	return &schemeTransport{
		scheme: "https",
		Transport: &http.Transport{
			Protocols:           protocols,
			TLSClientConfig:     tlsCfg,
			DialContext:         (&net.Dialer{Timeout: cfg.DialTimeout}).DialContext,
			TLSHandshakeTimeout: cfg.DialTimeout,
		},
	}, nil
}

// Native protocol selection can fall back to HTTP/1 for a mismatched URL scheme. Network failures are marked with
// ErrTransport.
type schemeTransport struct {
	*http.Transport
	scheme string
}

func (t *schemeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != t.scheme {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("connect client: transport requires %s URLs", t.scheme)
	}
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return nil, markTransport(req, err)
	}
	resp.Body = &transportBody{ReadCloser: resp.Body, req: req}
	return resp, nil
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
