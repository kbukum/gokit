package client

import (
	"fmt"
	"time"
)

// Config holds configuration for a Connect client connection.
type Config struct {
	// BaseURL is the target server URL (e.g. "http://localhost:8080").
	BaseURL string `yaml:"base_url" mapstructure:"base_url"`

	// Timeout is the HTTP request timeout. Zero means no timeout.
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// DialTimeout is the timeout for establishing the TCP connection.
	DialTimeout time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// Protocol selects the wire protocol: "connect" (default), "grpc", or "grpcweb".
	// Use "grpc" when the server only speaks gRPC or when using bidi streaming.
	Protocol string `yaml:"protocol" mapstructure:"protocol"`
}

const (
	defaultTimeout     = 30 * time.Second
	defaultDialTimeout = 10 * time.Second

	// ProtocolConnect is the default ConnectRPC protocol.
	ProtocolConnect = "connect"
	// ProtocolGRPC uses the gRPC wire protocol (required for bidi streaming).
	ProtocolGRPC = "grpc"
	// ProtocolGRPCWeb uses the gRPC-Web wire protocol.
	ProtocolGRPCWeb = "grpcweb"
)

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.Protocol == "" {
		c.Protocol = ProtocolConnect
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("connect client: base_url must not be empty")
	}
	switch c.Protocol {
	case ProtocolConnect, ProtocolGRPC, ProtocolGRPCWeb:
		// valid
	default:
		return fmt.Errorf("connect client: unsupported protocol %q (use connect, grpc, or grpcweb)", c.Protocol)
	}
	return nil
}
