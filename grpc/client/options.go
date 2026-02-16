package client

import (
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/grpc/interceptor"
	"github.com/kbukum/gokit/logger"
)

// ClientOptionsBuilder constructs gRPC dial options from configuration.
// Provides a fluent interface for building options with best practices.
type ClientOptionsBuilder struct {
	config             *grpccfg.Config
	enableLogging      bool
	enableMetrics      bool
	retryPolicy        *RetryPolicy
	customInterceptors []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
}

// RetryPolicy configures the gRPC retry behavior via service config.
type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoff    string // e.g. "0.1s"
	MaxBackoff        string // e.g. "1s"
	BackoffMultiplier float64
	WaitForReady      bool
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:       4,
		InitialBackoff:    "0.1s",
		MaxBackoff:        "1s",
		BackoffMultiplier: 2.0,
		WaitForReady:      true,
	}
}

// NewClientOptionsBuilder creates a new options builder from gokit gRPC config.
func NewClientOptionsBuilder(cfg *grpccfg.Config) *ClientOptionsBuilder {
	return &ClientOptionsBuilder{
		config:        cfg,
		enableLogging: true,
		enableMetrics: false,
		retryPolicy:   DefaultRetryPolicy(),
	}
}

// WithLogging enables or disables the logging interceptor.
func (b *ClientOptionsBuilder) WithLogging(enabled bool) *ClientOptionsBuilder {
	b.enableLogging = enabled
	return b
}

// WithRetryPolicy sets a custom retry policy.
func (b *ClientOptionsBuilder) WithRetryPolicy(p *RetryPolicy) *ClientOptionsBuilder {
	b.retryPolicy = p
	return b
}

// WithUnaryInterceptor adds a custom unary client interceptor.
func (b *ClientOptionsBuilder) WithUnaryInterceptor(i grpc.UnaryClientInterceptor) *ClientOptionsBuilder {
	b.customInterceptors = append(b.customInterceptors, i)
	return b
}

// WithStreamInterceptor adds a custom stream client interceptor.
func (b *ClientOptionsBuilder) WithStreamInterceptor(i grpc.StreamClientInterceptor) *ClientOptionsBuilder {
	b.streamInterceptors = append(b.streamInterceptors, i)
	return b
}

// Build constructs the complete set of dial options.
func (b *ClientOptionsBuilder) Build() []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                b.config.Keepalive.Time,
			Timeout:             b.config.Keepalive.Timeout,
			PermitWithoutStream: b.config.Keepalive.PermitWithoutStream,
		}),
	}

	if b.retryPolicy != nil {
		opts = append(opts, grpc.WithDefaultServiceConfig(b.buildServiceConfig()))
	}

	// Unary interceptors
	unary := b.buildUnaryInterceptors()
	if len(unary) > 0 {
		opts = append(opts, grpc.WithChainUnaryInterceptor(unary...))
	}

	// Stream interceptors
	stream := b.buildStreamInterceptors()
	if len(stream) > 0 {
		opts = append(opts, grpc.WithChainStreamInterceptor(stream...))
	}

	return opts
}

func (b *ClientOptionsBuilder) buildServiceConfig() string {
	p := b.retryPolicy
	return fmt.Sprintf(`{
		"methodConfig": [{
			"name": [{"service": ""}],
			"waitForReady": %t,
			"retryPolicy": {
				"maxAttempts": %d,
				"initialBackoff": "%s",
				"maxBackoff": "%s",
				"backoffMultiplier": %.1f,
				"retryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED", "ABORTED"]
			}
		}],
		"loadBalancingConfig": [{"round_robin": {}}]
	}`, p.WaitForReady, p.MaxAttempts, p.InitialBackoff, p.MaxBackoff, p.BackoffMultiplier)
}

func (b *ClientOptionsBuilder) buildUnaryInterceptors() []grpc.UnaryClientInterceptor {
	var interceptors []grpc.UnaryClientInterceptor

	if b.config.CallTimeout > 0 {
		interceptors = append(interceptors, interceptor.UnaryClientTimeoutInterceptor(b.config.CallTimeout))
	}
	if b.enableLogging {
		log := logger.NewDefault("grpc-client")
		interceptors = append(interceptors, interceptor.UnaryClientLoggingInterceptor(log))
	}
	interceptors = append(interceptors, b.customInterceptors...)
	return interceptors
}

func (b *ClientOptionsBuilder) buildStreamInterceptors() []grpc.StreamClientInterceptor {
	var interceptors []grpc.StreamClientInterceptor
	if b.enableLogging {
		log := logger.NewDefault("grpc-client")
		interceptors = append(interceptors, interceptor.StreamClientLoggingInterceptor(log))
	}
	interceptors = append(interceptors, b.streamInterceptors...)
	return interceptors
}

// GetDialTimeout returns the configured call timeout as dial timeout.
func (b *ClientOptionsBuilder) GetDialTimeout() time.Duration {
	if b.config.CallTimeout > 0 {
		return b.config.CallTimeout
	}
	return 10 * time.Second
}
