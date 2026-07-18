package discovery

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	disc "github.com/kbukum/gokit/discovery"
	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/grpc/client"
	"github.com/kbukum/gokit/logging"
)

// ResolverConnectionFactory creates gRPC connections using the native gRPC resolver. Unlike DiscoveryConnectionFactory (which resolves once), this factory creates connections that dynamically resolve addresses via Consul/discovery throughout their lifetime.
type ResolverConnectionFactory struct {
	builder  *ResolverBuilder
	gRPCCfg  grpccfg.Config
	log      *logging.Logger
	dialOpts []grpc.DialOption
}

// NewResolverConnectionFactory creates a factory that uses gRPC's native resolver for dynamic service discovery.
func NewResolverConnectionFactory(
	discovery disc.Discovery,
	gRPCCfg grpccfg.Config,
	log *logging.Logger,
	resolverOpts ...Option,
) *ResolverConnectionFactory {
	return &ResolverConnectionFactory{
		builder: NewResolverBuilder(discovery, log, resolverOpts...),
		gRPCCfg: gRPCCfg,
		log:     log,
	}
}

// NewConn creates a gRPC connection that dynamically resolves addresses. The serviceName is used as the resolver target (e.g., "consul:///ssm-ingestion").
func (f *ResolverConnectionFactory) NewConn(serviceName string) (*grpc.ClientConn, error) {
	target := f.builder.Scheme() + ":///" + serviceName

	f.log.Debug("Creating gRPC connection with discovery resolver", map[string]any{
		"service": serviceName,
		"target":  target,
	})

	opts, err := f.buildDialOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to build dial options: %w", err)
	}

	opts = append(opts, f.dialOpts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s at %s: %w", serviceName, target, err)
	}

	return conn, nil
}

// buildDialOptions builds the gRPC dial options from the factory's configuration. This mirrors the logic in DiscoveryConnectionFactory to ensure consistent behavior.
func (f *ResolverConnectionFactory) buildDialOptions() ([]grpc.DialOption, error) {
	f.gRPCCfg.ApplyDefaults()

	if err := f.gRPCCfg.Validate(); err != nil {
		return nil, fmt.Errorf("grpc config: %w", err)
	}

	opts := make([]grpc.DialOption, 0, 6)

	// Transport credentials
	creds, err := f.transportCredentials()
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		grpc.WithResolvers(f.builder),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                f.gRPCCfg.Keepalive.Time,
			Timeout:             f.gRPCCfg.Keepalive.Timeout,
			PermitWithoutStream: f.gRPCCfg.Keepalive.PermitWithoutStream,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(f.gRPCCfg.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(f.gRPCCfg.MaxSendMsgSize),
		),
	)

	return opts, nil
}

// transportCredentials returns the appropriate transport credentials.
func (f *ResolverConnectionFactory) transportCredentials() (credentials.TransportCredentials, error) {
	tlsCfg, err := f.gRPCCfg.TLS.Build()
	if err != nil {
		return nil, fmt.Errorf("grpc tls: %w", err)
	}

	if tlsCfg == nil {
		return insecure.NewCredentials(), nil
	}

	return credentials.NewTLS(tlsCfg), nil
}

// Compile-time check.
var _ client.ConnectionFactory = (*ResolverConnectionFactory)(nil)
