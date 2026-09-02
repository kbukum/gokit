package grpc

import (
	"fmt"
	"time"

	"github.com/kbukum/gokit/resilience"
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
	// Target is the gRPC server target (e.g., "localhost:50051"). Set statically via config
	// or dynamically via discovery (endpoint.HostPort()).
	Target string `mapstructure:"target"`
	// MaxMessageSize is the maximum message size the client can receive (bytes).
	MaxMessageSize int `mapstructure:"max_message_size"`
	// MaxSendMessageSize is the maximum message size the client can send (bytes).
	MaxSendMessageSize int `mapstructure:"max_send_message_size"`
	// Keepalive holds keepalive configuration.
	Keepalive KeepaliveConfig `mapstructure:"keepalive"`
	// TLS holds TLS configuration.
	// The shared security policy enforces a TLS 1.2 floor while default negotiation still prefers TLS 1.3.
	TLS *security.TLSConfig `mapstructure:"tls"`
	// Enabled controls whether gRPC is active.
	Enabled bool `mapstructure:"enabled"`
	// Timeout is the default timeout for unary RPCs.
	Timeout time.Duration `mapstructure:"timeout"`
	// ResiliencePolicy configures the retry / circuit-breaker / rate-limiter
	// stack applied to unary RPCs via the client resilience interceptor. Nil
	// applies timeout-only behavior derived from Timeout. It uses the shared
	// resilience.Policy vocabulary, the same block understood by the httpclient
	// transport.
	ResiliencePolicy *resilience.Policy `mapstructure:"resilience_policy"`
}

const (
	defaultTarget           = "localhost:50051"
	defaultMaxMessageSize   = 4 * 1024 * 1024 // 4 MB
	defaultKeepaliveTime    = 30 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second
	defaultTimeout          = 30 * time.Second
)

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Target == "" {
		c.Target = defaultTarget
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = defaultMaxMessageSize
	}
	if c.MaxSendMessageSize == 0 {
		c.MaxSendMessageSize = defaultMaxMessageSize
	}
	if c.Keepalive.Time == 0 {
		c.Keepalive.Time = defaultKeepaliveTime
	}
	if c.Keepalive.Timeout == 0 {
		c.Keepalive.Timeout = defaultKeepaliveTimeout
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Target == "" {
		return fmt.Errorf("grpc: target must not be empty")
	}
	if c.MaxMessageSize <= 0 {
		return fmt.Errorf("grpc: max_message_size must be positive, got %d", c.MaxMessageSize)
	}
	if c.MaxSendMessageSize <= 0 {
		return fmt.Errorf("grpc: max_send_message_size must be positive, got %d", c.MaxSendMessageSize)
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
	return c.Target
}
