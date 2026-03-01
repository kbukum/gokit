package client

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/provider"
)

// compile-time assertions
var _ provider.Provider = (*Adapter)(nil)
var _ provider.Closeable = (*Adapter)(nil)

// Adapter manages a gRPC connection and implements the provider.Provider interface.
// Unlike HTTP, gRPC adapters don't do request mapping — proto handles types.
// The adapter manages connection lifecycle and provides the conn for proto stubs.
type Adapter struct {
	config grpccfg.Config
	conn   *grpc.ClientConn
	log    *logger.Logger
}

// AdapterOption configures an Adapter during creation.
type AdapterOption func(*Adapter)

// NewAdapter creates a gRPC adapter with a managed connection.
func NewAdapter(cfg grpccfg.Config, log *logger.Logger, opts ...AdapterOption) (*Adapter, error) {
	conn, err := NewClient(cfg, log)
	if err != nil {
		return nil, err
	}

	a := &Adapter{
		config: cfg,
		conn:   conn,
		log:    log,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Conn returns the managed gRPC connection.
// Use with generated proto client stubs.
func (a *Adapter) Conn() *grpc.ClientConn {
	return a.conn
}

// Name returns the adapter name (implements provider.Provider).
func (a *Adapter) Name() string {
	return a.config.Name
}

// IsAvailable checks if the connection is ready or idle (implements provider.Provider).
func (a *Adapter) IsAvailable(_ context.Context) bool {
	if a.conn == nil {
		return false
	}
	state := a.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Close closes the gRPC connection (implements provider.Closeable).
func (a *Adapter) Close(_ context.Context) error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// GetConfig returns the adapter's configuration.
func (a *Adapter) GetConfig() grpccfg.Config {
	return a.config
}

// ClientOf creates a typed gRPC client from the adapter's connection.
//
// Usage:
//
//	userClient := client.ClientOf(adapter, pb.NewUserServiceClient)
func ClientOf[T any](a *Adapter, newClient func(grpc.ClientConnInterface) T) T {
	return newClient(a.conn)
}
