package client

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kbukum/gokit/security"
)

// Config holds configuration for a Connect client connection.
type Config struct {
	// BaseURL is the root URL for the target service (e.g., "http://localhost:8080"), optionally with a path prefix. Its scheme matches the transport: https with TLS, http (h2c) without. It carries no credentials, query or fragment. Optional when using service discovery, which resolves addresses dynamically.
	BaseURL string `yaml:"base_url" mapstructure:"base_url"`

	// Timeout bounds each whole request, including reading a streamed response. Zero uses the default; set NoTimeout for long-lived streams.
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// NoTimeout builds a client without a whole-request timeout, for streams that stay open as long as the caller listens. Bound each call with its context instead. It cannot be combined with Timeout.
	NoTimeout bool `yaml:"no_timeout" mapstructure:"no_timeout"`

	// DialTimeout is the timeout for establishing the TCP connection.
	DialTimeout time.Duration `yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// Protocol selects the wire protocol: "connect" (default), "grpc", or "grpcweb".
	// Use "grpc" when the server only speaks gRPC or when using bidi streaming.
	Protocol string `yaml:"protocol" mapstructure:"protocol"`

	// TLS configures TLS for the connection. When nil, h2c (cleartext HTTP/2) is used. When set,
	// standard HTTPS/TLS transport is used instead.
	TLS *security.TLSConfig `yaml:"tls" mapstructure:"tls"`
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
	if c.Timeout == 0 && !c.NoTimeout {
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
	switch c.Protocol {
	case ProtocolConnect, ProtocolGRPC, ProtocolGRPCWeb:
		// valid
	default:
		return fmt.Errorf("connect client: unsupported protocol %q (use connect, grpc, or grpcweb)", c.Protocol)
	}
	if c.Timeout < 0 {
		return fmt.Errorf("connect client: timeout must not be negative; use no_timeout for an unbounded client")
	}
	if c.DialTimeout < 0 {
		return fmt.Errorf("connect client: dial_timeout must not be negative")
	}
	if c.NoTimeout && c.Timeout != 0 {
		return fmt.Errorf("connect client: timeout and no_timeout are mutually exclusive")
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}
	return c.validateBaseURL()
}

// validateBaseURL never echoes the URL, which may carry a credential.
func (c *Config) validateBaseURL() error {
	if c.BaseURL == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return errors.New("connect client: base_url does not parse")
	}
	switch {
	case u.User != nil:
		return errors.New("connect client: base_url must not carry credentials")
	case u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https"):
		return errors.New("connect client: base_url must be an absolute http or https URL")
	// url.Parse drops an empty fragment, so look for the raw delimiter; an escaped %23 stays valid.
	case u.RawQuery != "" || u.ForceQuery || strings.Contains(c.BaseURL, "#"):
		return errors.New("connect client: base_url must not have a query or fragment")
	case u.Scheme == "https" && !c.TLS.IsEnabled():
		return errors.New("connect client: an https base_url needs tls settings")
	case u.Scheme == "http" && c.TLS.IsEnabled():
		return errors.New("connect client: an http base_url is cleartext h2c; use https with tls settings")
	}
	return nil
}
