package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/logging"

	"github.com/kbukum/gokit/discovery"
)

// DiscoveryConnectionFactory creates gRPC connections using service discovery.
// It resolves service names to addresses via a Discovery client and creates connections
// to the resolved endpoints. This enables dynamic service discovery with load balancing.
type DiscoveryConnectionFactory struct {
	discoveryClient *discovery.Client
	gRPCCfg         grpccfg.Config
	log             *logging.Logger
	dialOpts        []grpc.DialOption
}

// NewDiscoveryConnectionFactory creates a new discovery-based connection factory.
//
// Parameters:
//   - discoveryClient: The discovery client to use for resolving service addresses.
//     This client handles caching, load balancing, and fallback endpoints.
//   - gRPCCfg: Base gRPC configuration for TLS, keepalive, message sizes, and timeouts.
//   - log: Optional logger; if nil, uses the global logging.
//   - opts: Additional gRPC dial options to apply when creating connections.
//
// The factory will use the discovery client's load balancing strategy to select
// among healthy service endpoints. All connections use the same TLS and keepalive
// configuration from gRPCCfg.
func NewDiscoveryConnectionFactory(
	discoveryClient *discovery.Client,
	gRPCCfg grpccfg.Config,
	log *logging.Logger,
	opts ...grpc.DialOption,
) *DiscoveryConnectionFactory {
	return &DiscoveryConnectionFactory{
		discoveryClient: discoveryClient,
		gRPCCfg:         gRPCCfg,
		log:             log,
		dialOpts:        opts,
	}
}

// NewConn creates a new gRPC client connection to a discovered service.
// It uses the discovery client to resolve the service name to a healthy endpoint,
// then creates a connection with the configured TLS and keepalive settings.
//
// Parameters:
//   - serviceName: The name of the service to discover and connect to.
//
// Returns the created connection or an error if discovery fails or the connection
// cannot be established.
func (f *DiscoveryConnectionFactory) NewConn(serviceName string) (*grpc.ClientConn, error) {
	// Use background context for service discovery
	ctx := context.Background()

	// Discover one healthy endpoint using the discovery client's load balancing
	query := discovery.Query{
		ServiceName: serviceName,
		Strategy:    discovery.StrategyRoundRobin,
	}
	instance, err := f.discoveryClient.DiscoverOne(ctx, query)
	if err != nil {
		f.logError("Failed to discover service", map[string]interface{}{
			"service": serviceName,
			"error":   err.Error(),
		})
		return nil, fmt.Errorf("failed to discover service %q: %w", serviceName, err)
	}

	// Build target address from the discovered instance
	target := instance.Address + ":" + fmt.Sprint(instance.Port)

	f.logDebug("Creating gRPC connection via discovery", map[string]interface{}{
		"service":  serviceName,
		"target":   target,
		"instance": instance.ID,
		"health":   instance.Health,
	})

	// Build dial options from config
	opts, err := f.buildDialOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to build dial options: %w", err)
	}

	// Append any additional user-provided options
	opts = append(opts, f.dialOpts...)

	// Create the gRPC connection
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		f.logError("Failed to create gRPC connection", map[string]interface{}{
			"service": serviceName,
			"target":  target,
			"error":   err.Error(),
		})
		return nil, fmt.Errorf("failed to connect to %s at %s: %w", serviceName, target, err)
	}

	f.logDebug("gRPC connection created via discovery", map[string]interface{}{
		"service": serviceName,
		"target":  target,
	})

	return conn, nil
}

// buildDialOptions builds the gRPC dial options from the factory's configuration.
// This mirrors the logic in client.go to ensure consistent behavior between
// direct and discovery-based connections.
func (f *DiscoveryConnectionFactory) buildDialOptions() ([]grpc.DialOption, error) {
	f.gRPCCfg.ApplyDefaults()

	// Validate config
	if err := f.gRPCCfg.Validate(); err != nil {
		return nil, fmt.Errorf("grpc config: %w", err)
	}

	opts := make([]grpc.DialOption, 0, 5)

	// Transport credentials
	creds, err := f.transportCredentials()
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		grpc.WithTransportCredentials(creds),
		// Keepalive configuration
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                f.gRPCCfg.Keepalive.Time,
			Timeout:             f.gRPCCfg.Keepalive.Timeout,
			PermitWithoutStream: f.gRPCCfg.Keepalive.PermitWithoutStream,
		}),
		// Message size limits
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(f.gRPCCfg.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(f.gRPCCfg.MaxSendMsgSize),
		),
	)

	return opts, nil
}

// transportCredentials returns the appropriate transport credentials for TLS.
// This mirrors the logic in client.go.
func (f *DiscoveryConnectionFactory) transportCredentials() (credentials.TransportCredentials, error) {
	tlsCfg, err := f.gRPCCfg.TLS.Build()
	if err != nil {
		return nil, fmt.Errorf("grpc tls: %w", err)
	}

	if tlsCfg == nil {
		return insecure.NewCredentials(), nil
	}

	return credentials.NewTLS(tlsCfg), nil
}

func (f *DiscoveryConnectionFactory) logDebug(msg string, fields ...map[string]interface{}) {
	if f.log != nil {
		f.log.Debug(msg, fields...)
	} else {
		logging.Debug(msg, fields...)
	}
}

func (f *DiscoveryConnectionFactory) logError(msg string, fields ...map[string]interface{}) {
	if f.log != nil {
		f.log.Error(msg, fields...)
	} else {
		logging.Error(msg, fields...)
	}
}

// Compile-time check that DiscoveryConnectionFactory implements ConnectionFactory.
var _ ConnectionFactory = (*DiscoveryConnectionFactory)(nil)
