package grpc

import (
	"fmt"
	"time"

	"github.com/kbukum/gokit/security"
)

// KeepaliveConfig holds keepalive settings for gRPC connections.
type KeepaliveConfig struct {
	// Time is the interval between keepalive pings.
	Time time.Duration `mapstructure:"time"`
	// Timeout is the time to wait for a keepalive ping ack before closing.
	Timeout time.Duration `mapstructure:"timeout"`
	// PermitWithoutStream allows keepalive pings when there are no active RPCs.
	PermitWithoutStream bool `mapstructure:"permit_without_stream"`
}

// Config holds configuration for a gRPC client connection.
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `mapstructure:"name"`
	// BaseURL is the gRPC server target (e.g. "localhost:50051" or "dns:///myservice").
	// Optional when using service discovery, which resolves targets dynamically.
	BaseURL string `mapstructure:"base_url"`
	// MaxRecvMsgSize is the maximum message size the client can receive (bytes).
	MaxRecvMsgSize int `mapstructure:"max_recv_msg_size"`
	// MaxSendMsgSize is the maximum message size the client can send (bytes).
	MaxSendMsgSize int `mapstructure:"max_send_msg_size"`
	// Keepalive holds keepalive configuration.
	Keepalive KeepaliveConfig `mapstructure:"keepalive"`
	// TLS holds TLS configuration.
	TLS *security.TLSConfig `mapstructure:"tls"`
	// Enabled controls whether gRPC is active.
	Enabled bool `mapstructure:"enabled"`
	// CallTimeout is the default timeout for unary RPCs.
	CallTimeout time.Duration `mapstructure:"call_timeout"`
}

const (
	defaultBaseURL              = "localhost:50051"
	defaultMaxRecvMsgSize   = 4 * 1024 * 1024 // 4 MB
	defaultMaxSendMsgSize   = 4 * 1024 * 1024 // 4 MB
	defaultKeepaliveTime    = 30 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second
	defaultCallTimeout      = 30 * time.Second
)

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.MaxRecvMsgSize == 0 {
		c.MaxRecvMsgSize = defaultMaxRecvMsgSize
	}
	if c.MaxSendMsgSize == 0 {
		c.MaxSendMsgSize = defaultMaxSendMsgSize
	}
	if c.Keepalive.Time == 0 {
		c.Keepalive.Time = defaultKeepaliveTime
	}
	if c.Keepalive.Timeout == 0 {
		c.Keepalive.Timeout = defaultKeepaliveTimeout
	}
	if c.CallTimeout == 0 {
		c.CallTimeout = defaultCallTimeout
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return fmt.Errorf("grpc: base_url must not be empty")
	}
	if c.MaxRecvMsgSize <= 0 {
		return fmt.Errorf("grpc: max_recv_msg_size must be positive, got %d", c.MaxRecvMsgSize)
	}
	if c.MaxSendMsgSize <= 0 {
		return fmt.Errorf("grpc: max_send_msg_size must be positive, got %d", c.MaxSendMsgSize)
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("grpc: %w", err)
		}
	}
	return nil
}

// Address returns the dial target for gRPC connections.
func (c *Config) Address() string {
	return c.BaseURL
}
