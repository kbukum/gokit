package client

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/grpc/interceptor"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

// NewClient creates a gRPC client connection using the provided configuration and logging.
// It configures keepalive, TLS, message size limits, and attaches logging
// and resilience interceptors.
func NewClient(cfg grpccfg.Config, log *logging.Logger) (*grpc.ClientConn, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("grpc client config: %w", err)
	}

	target := cfg.Address()

	log.Info("Connecting to gRPC server", map[string]any{
		"target": target,
		"tls":    cfg.TLS.IsEnabled(),
	})

	opts, err := buildDialOptions(cfg, log)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		log.Error("Failed to create gRPC client", map[string]any{
			"target": target,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("grpc: failed to create client for %s: %w", target, err)
	}

	log.Info("gRPC client created", map[string]any{
		"target": target,
	})

	return conn, nil
}

// buildDialOptions assembles all gRPC dial options from config.
func buildDialOptions(cfg grpccfg.Config, log *logging.Logger) ([]grpc.DialOption, error) {
	opts := make([]grpc.DialOption, 0, 5)

	// Transport credentials
	creds, err := transportCredentials(cfg.TLS)
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		grpc.WithTransportCredentials(creds),
		// Keepalive
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.Keepalive.Time,
			Timeout:             cfg.Keepalive.Timeout,
			PermitWithoutStream: cfg.Keepalive.PermitWithoutStream,
		}),
		// Message size limits
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(cfg.MaxMessageSize),
			grpc.MaxCallSendMsgSize(cfg.MaxSendMessageSize),
		),
	)

	// Unary interceptors: logging → resilience
	unary := []grpc.UnaryClientInterceptor{interceptor.UnaryClientLoggingInterceptor(log)}
	if policy := resiliencePolicyFor(cfg); policy != nil {
		unary = append(unary, interceptor.UnaryClientResilienceInterceptor(policy))
	}
	opts = append(opts,
		grpc.WithChainUnaryInterceptor(unary...),
		// Stream interceptors: logging
		grpc.WithChainStreamInterceptor(
			interceptor.StreamClientLoggingInterceptor(log),
		),
	)

	return opts, nil
}

// resiliencePolicyFor returns the unary resilience policy for a client config.
// An explicit ResiliencePolicy is used as-is (its retry block, if present,
// defaults to the gRPC-aware IsRetryable predicate); otherwise a timeout-only
// policy is derived from Timeout, and nil is returned when neither is set.
func resiliencePolicyFor(cfg grpccfg.Config) *resilience.Policy {
	if p := cfg.ResiliencePolicy; p != nil {
		if p.Retry != nil && p.Retry.RetryIf == nil {
			p.Retry.RetryIf = interceptor.IsRetryable
		}
		if p.Timeout == 0 && cfg.Timeout > 0 {
			p.WithTimeoutIfUnset(cfg.Timeout)
		}
		return p
	}
	if cfg.Timeout > 0 {
		return resilience.NewPolicy().WithTimeoutIfUnset(cfg.Timeout)
	}
	return nil
}

// transportCredentials returns the appropriate transport credentials.
func transportCredentials(cfg *security.TLSConfig) (credentials.TransportCredentials, error) {
	tlsCfg, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("grpc: %w", err)
	}
	if tlsCfg == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(tlsCfg), nil
}
