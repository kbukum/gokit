package client

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/grpc/interceptor"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
)

// ClientOptionsBuilder constructs gRPC dial options from configuration.
type ClientOptionsBuilder struct {
	config             *grpccfg.Config
	enableLogging      bool
	resiliencePolicy   *resilience.Policy
	customInterceptors []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
}

// DefaultRetryPolicy returns a retry policy aligned with gRPC retryable status codes.
func DefaultRetryPolicy() *resilience.RetryConfig {
	cfg := resilience.DefaultRetryConfig()
	cfg.MaxAttempts = 4
	cfg.MaxBackoff = time.Second
	cfg.RetryIf = interceptor.IsRetryable
	return &cfg
}

// NewClientOptionsBuilder creates a new options builder from gokit gRPC config.
// A ResiliencePolicy set on the config is honored as the client's policy;
// otherwise the builder seeds a timeout-plus-default-retry policy.
func NewClientOptionsBuilder(cfg *grpccfg.Config) *ClientOptionsBuilder {
	policy := cfg.ResiliencePolicy
	if policy == nil {
		policy = resilience.NewPolicy().WithTimeoutIfUnset(cfg.Timeout).WithRetry(*DefaultRetryPolicy())
	} else if policy.Timeout == 0 && cfg.Timeout > 0 {
		policy.WithTimeoutIfUnset(cfg.Timeout)
	}
	return &ClientOptionsBuilder{
		config:           cfg,
		enableLogging:    true,
		resiliencePolicy: policy,
	}
}

// WithLogging enables or disables the logging interceptor.
func (b *ClientOptionsBuilder) WithLogging(enabled bool) *ClientOptionsBuilder {
	b.enableLogging = enabled
	return b
}

// WithRetryPolicy sets or disables the retry portion of the client resilience policy.
func (b *ClientOptionsBuilder) WithRetryPolicy(cfg *resilience.RetryConfig) *ClientOptionsBuilder {
	if b.resiliencePolicy == nil {
		b.resiliencePolicy = resilience.NewPolicy().WithTimeoutIfUnset(b.config.Timeout)
	}
	if cfg == nil {
		b.resiliencePolicy.Retry = nil
		return b
	}
	copyCfg := *cfg
	b.resiliencePolicy.Retry = &copyCfg
	return b
}

// WithResiliencePolicy replaces the entire client resilience policy.
func (b *ClientOptionsBuilder) WithResiliencePolicy(policy *resilience.Policy) *ClientOptionsBuilder {
	b.resiliencePolicy = policy
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
func (b *ClientOptionsBuilder) Build() ([]grpc.DialOption, error) {
	creds, err := transportCredentials(b.config.TLS)
	if err != nil {
		return nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                b.config.Keepalive.Time,
			Timeout:             b.config.Keepalive.Timeout,
			PermitWithoutStream: b.config.Keepalive.PermitWithoutStream,
		}),
	}

	unary := b.buildUnaryInterceptors()
	if len(unary) > 0 {
		opts = append(opts, grpc.WithChainUnaryInterceptor(unary...))
	}

	stream := b.buildStreamInterceptors()
	if len(stream) > 0 {
		opts = append(opts, grpc.WithChainStreamInterceptor(stream...))
	}

	return opts, nil
}

func (b *ClientOptionsBuilder) buildUnaryInterceptors() []grpc.UnaryClientInterceptor {
	interceptors := make([]grpc.UnaryClientInterceptor, 0, len(b.customInterceptors)+2)

	if b.enableLogging {
		log := logging.NewDefault("grpc-client")
		interceptors = append(interceptors, interceptor.UnaryClientLoggingInterceptor(log))
	}
	if b.resiliencePolicy != nil {
		interceptors = append(interceptors, interceptor.UnaryClientResilienceInterceptor(b.resiliencePolicy))
	}
	interceptors = append(interceptors, b.customInterceptors...)
	return interceptors
}

func (b *ClientOptionsBuilder) buildStreamInterceptors() []grpc.StreamClientInterceptor {
	var interceptors []grpc.StreamClientInterceptor
	if b.enableLogging {
		log := logging.NewDefault("grpc-client")
		interceptors = append(interceptors, interceptor.StreamClientLoggingInterceptor(log))
	}
	interceptors = append(interceptors, b.streamInterceptors...)
	return interceptors
}

// GetDialTimeout returns the configured call timeout as dial timeout.
func (b *ClientOptionsBuilder) GetDialTimeout() time.Duration {
	if b.config.Timeout > 0 {
		return b.config.Timeout
	}
	return 10 * time.Second
}
