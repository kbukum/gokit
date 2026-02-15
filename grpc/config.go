package grpc

import (
	"fmt"
	"time"
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

// TLSConfig holds TLS settings for gRPC connections.
type TLSConfig struct {
	// Enabled enables TLS for the connection.
	Enabled bool `mapstructure:"enabled"`
	// CertFile is the path to the TLS certificate file.
	CertFile string `mapstructure:"cert_file"`
	// KeyFile is the path to the TLS key file.
	KeyFile string `mapstructure:"key_file"`
	// CAFile is the path to the CA certificate file for verifying the server.
	CAFile string `mapstructure:"ca_file"`
	// InsecureSkipVerify disables server certificate verification.
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// Config holds configuration for a gRPC client connection.
type Config struct {
	// Host is the gRPC server hostname.
	Host string `mapstructure:"host"`
	// Port is the gRPC server port.
	Port int `mapstructure:"port"`
	// MaxRecvMsgSize is the maximum message size the client can receive (bytes).
	MaxRecvMsgSize int `mapstructure:"max_recv_msg_size"`
	// MaxSendMsgSize is the maximum message size the client can send (bytes).
	MaxSendMsgSize int `mapstructure:"max_send_msg_size"`
	// Keepalive holds keepalive configuration.
	Keepalive KeepaliveConfig `mapstructure:"keepalive"`
	// TLS holds TLS configuration.
	TLS TLSConfig `mapstructure:"tls"`
	// Enabled controls whether gRPC is active.
	Enabled bool `mapstructure:"enabled"`
	// CallTimeout is the default timeout for unary RPCs.
	CallTimeout time.Duration `mapstructure:"call_timeout"`
}

const (
	defaultHost           = "localhost"
	defaultPort           = 50051
	defaultMaxRecvMsgSize = 4 * 1024 * 1024 // 4 MB
	defaultMaxSendMsgSize = 4 * 1024 * 1024 // 4 MB
	defaultKeepaliveTime  = 30 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second
	defaultCallTimeout    = 30 * time.Second
)

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Host == "" {
		c.Host = defaultHost
	}
	if c.Port == 0 {
		c.Port = defaultPort
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
	if c.Host == "" {
		return fmt.Errorf("grpc: host must not be empty")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("grpc: port must be between 1 and 65535, got %d", c.Port)
	}
	if c.MaxRecvMsgSize <= 0 {
		return fmt.Errorf("grpc: max_recv_msg_size must be positive, got %d", c.MaxRecvMsgSize)
	}
	if c.MaxSendMsgSize <= 0 {
		return fmt.Errorf("grpc: max_send_msg_size must be positive, got %d", c.MaxSendMsgSize)
	}
	if c.TLS.Enabled {
		if c.TLS.CertFile == "" {
			return fmt.Errorf("grpc: tls cert_file is required when TLS is enabled")
		}
		if c.TLS.KeyFile == "" {
			return fmt.Errorf("grpc: tls key_file is required when TLS is enabled")
		}
	}
	return nil
}

// Address returns the host:port dial target.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
