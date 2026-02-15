package client

import (
	"google.golang.org/grpc"

	grpccfg "github.com/skillsenselab/gokit/grpc"
	"github.com/skillsenselab/gokit/logger"
)

// ConnectionFactory defines the interface for creating gRPC connections.
// This abstraction allows for different connection strategies (direct, discovery-based, etc.)
type ConnectionFactory interface {
	// NewConn creates a new gRPC client connection to the specified service.
	// The connection is created lazily by grpc.NewClient and connects on first RPC.
	NewConn(serviceName string) (*grpc.ClientConn, error)
}

// DefaultConnectionFactory creates gRPC connections using gokit Config.
type DefaultConnectionFactory struct {
	cfg grpccfg.Config
	log *logger.Logger
}

// NewDefaultConnectionFactory creates a factory that builds connections using the provided config and logger.
func NewDefaultConnectionFactory(cfg grpccfg.Config, log *logger.Logger) *DefaultConnectionFactory {
	return &DefaultConnectionFactory{cfg: cfg, log: log}
}

// NewConn creates a new gRPC client connection.
// The serviceName is used for logging; the target address comes from the factory's Config.
func (f *DefaultConnectionFactory) NewConn(serviceName string) (*grpc.ClientConn, error) {
	f.log.Debug("Creating gRPC connection via factory", map[string]interface{}{
		"service": serviceName,
		"target":  f.cfg.Address(),
	})
	return NewClient(f.cfg, f.log)
}
