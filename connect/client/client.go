package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
)

// NewHTTPClient creates an h2c-capable *http.Client for use with ConnectRPC.
//
// The returned client supports cleartext HTTP/2 (h2c) which is required for
// ConnectRPC and gRPC communication without TLS. It can be passed directly
// to any generated Connect client constructor.
func NewHTTPClient(cfg Config) *http.Client {
	cfg.ApplyDefaults()

	dialTimeout := cfg.DialTimeout

	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, addr)
			},
		},
	}
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
