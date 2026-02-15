package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	grpccfg "github.com/skillsenselab/gokit/grpc"
	"github.com/skillsenselab/gokit/grpc/interceptor"
	"github.com/skillsenselab/gokit/logger"
)

// NewClient creates a gRPC client connection using the provided configuration
// and logger. It configures keepalive, TLS, message size limits, and attaches
// logging and timeout interceptors.
func NewClient(cfg grpccfg.Config, log *logger.Logger) (*grpc.ClientConn, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("grpc client config: %w", err)
	}

	target := cfg.Address()

	log.Info("Connecting to gRPC server", map[string]interface{}{
		"target": target,
		"tls":    cfg.TLS.Enabled,
	})

	opts, err := buildDialOptions(cfg, log)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		log.Error("Failed to create gRPC client", map[string]interface{}{
			"target": target,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("grpc: failed to create client for %s: %w", target, err)
	}

	log.Info("gRPC client created", map[string]interface{}{
		"target": target,
	})

	return conn, nil
}

// buildDialOptions assembles all gRPC dial options from config.
func buildDialOptions(cfg grpccfg.Config, log *logger.Logger) ([]grpc.DialOption, error) {
	var opts []grpc.DialOption

	// Transport credentials
	creds, err := transportCredentials(cfg.TLS)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.WithTransportCredentials(creds))

	// Keepalive
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                cfg.Keepalive.Time,
		Timeout:             cfg.Keepalive.Timeout,
		PermitWithoutStream: cfg.Keepalive.PermitWithoutStream,
	}))

	// Message size limits
	opts = append(opts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(cfg.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(cfg.MaxSendMsgSize),
		),
	)

	// Unary interceptors: timeout → logging
	var unary []grpc.UnaryClientInterceptor
	if cfg.CallTimeout > 0 {
		unary = append(unary, interceptor.UnaryClientTimeoutInterceptor(cfg.CallTimeout))
	}
	unary = append(unary, interceptor.UnaryClientLoggingInterceptor(log))
	opts = append(opts, grpc.WithChainUnaryInterceptor(unary...))

	// Stream interceptors: logging
	opts = append(opts, grpc.WithChainStreamInterceptor(
		interceptor.StreamClientLoggingInterceptor(log),
	))

	return opts, nil
}

// transportCredentials returns the appropriate transport credentials.
func transportCredentials(cfg grpccfg.TLSConfig) (credentials.TransportCredentials, error) {
	if !cfg.Enabled {
		return insecure.NewCredentials(), nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("grpc: failed to load TLS key pair: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	if cfg.CAFile != "" {
		ca, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("grpc: failed to read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("grpc: failed to parse CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	return credentials.NewTLS(tlsCfg), nil
}
