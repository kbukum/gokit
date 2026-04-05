package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kbukum/gokit/discovery"
	grpccfg "github.com/kbukum/gokit/grpc"
)

// MockDiscovery is a mock implementation of discovery.Discovery.
type MockDiscovery struct {
	services map[string]discovery.ServiceInstance
	failure  bool
}

// NewMockDiscovery creates a new mock discovery client.
func NewMockDiscovery() *MockDiscovery {
	return &MockDiscovery{
		services: make(map[string]discovery.ServiceInstance),
	}
}

// WithService adds a service to the mock.
func (m *MockDiscovery) WithService(serviceName string, inst discovery.ServiceInstance) *MockDiscovery {
	m.services[serviceName] = inst
	return m
}

// WithFailure configures the mock to fail on discovery.
func (m *MockDiscovery) WithFailure(fail bool) *MockDiscovery {
	m.failure = fail
	return m
}

// Discover returns the registered service or an error.
func (m *MockDiscovery) Discover(ctx context.Context, serviceName string) ([]discovery.ServiceInstance, error) {
	if m.failure {
		return nil, fmt.Errorf("mock discovery failure")
	}
	if inst, ok := m.services[serviceName]; ok {
		return []discovery.ServiceInstance{inst}, nil
	}
	return nil, fmt.Errorf("service %q not found", serviceName)
}

// Watch is not implemented in this mock.
func (m *MockDiscovery) Watch(ctx context.Context, serviceName string) (<-chan []discovery.ServiceInstance, error) {
	return nil, fmt.Errorf("not implemented")
}

// Close is not implemented in this mock.
func (m *MockDiscovery) Close() error {
	return nil
}

// TestNewDiscoveryConnectionFactory tests factory creation.
func TestNewDiscoveryConnectionFactory(t *testing.T) {
	log := testLogger()
	mockDisc := NewMockDiscovery()

	cfg := grpccfg.Config{
		Addr:           "localhost:50051",
		MaxRecvMsgSize: 4 * 1024 * 1024,
		MaxSendMsgSize: 4 * 1024 * 1024,
	}

	// Create a real discovery.Client wrapping the mock
	dcfg := discovery.ClientConfig{CacheTTL: 30}
	dc := discovery.NewClient(mockDisc, dcfg, log)

	factory := NewDiscoveryConnectionFactory(dc, cfg, log)
	require.NotNil(t, factory, "Expected factory to be created")
	require.Equal(t, dc, factory.discoveryClient, "Discovery client not set correctly")
}

// TestDiscoveryConnectionFactoryDiscoveryFailure tests handling of discovery failure.
func TestDiscoveryConnectionFactoryDiscoveryFailure(t *testing.T) {
	log := testLogger()
	mockDisc := NewMockDiscovery().WithFailure(true)

	cfg := grpccfg.Config{
		Addr:           "localhost:50051",
		MaxRecvMsgSize: 4 * 1024 * 1024,
		MaxSendMsgSize: 4 * 1024 * 1024,
	}
	cfg.ApplyDefaults()

	// Create a real discovery.Client wrapping the mock
	dcfg := discovery.ClientConfig{
		CacheTTL: 30,
		Criticality: map[string]discovery.Criticality{
			"missing-service": discovery.CriticalityOptional,
		},
	}
	dc := discovery.NewClient(mockDisc, dcfg, log)

	factory := NewDiscoveryConnectionFactory(dc, cfg, log)

	// Try to create a connection to a non-existent service
	conn, err := factory.NewConn("missing-service")
	require.Error(t, err, "Expected discovery to fail for non-existent service")
	require.Nil(t, conn, "Expected conn to be nil on error")
}

// TestDiscoveryConnectionFactoryImplementsInterface tests that the factory
// implements the ConnectionFactory interface.
func TestDiscoveryConnectionFactoryImplementsInterface(t *testing.T) {
	log := testLogger()
	mockDisc := NewMockDiscovery()
	cfg := grpccfg.Config{}

	// Create a real discovery.Client wrapping the mock
	dcfg := discovery.ClientConfig{CacheTTL: 30}
	dc := discovery.NewClient(mockDisc, dcfg, log)

	factory := NewDiscoveryConnectionFactory(dc, cfg, log)

	// Check that factory implements ConnectionFactory
	var _ ConnectionFactory = factory
	require.NotNil(t, factory)
}

// BenchmarkDiscoveryConnectionFactoryNewConn benchmarks connection creation.
func BenchmarkDiscoveryConnectionFactoryNewConn(b *testing.B) {
	log := testLogger()
	mockDisc := NewMockDiscovery()

	inst := discovery.ServiceInstance{
		ID:       "bench-svc-1",
		Name:     "bench-service",
		Address:  "127.0.0.1",
		Port:     50051,
		Protocol: "grpc",
		Health:   discovery.HealthHealthy,
		Weight:   1,
	}
	mockDisc.WithService("bench-service", inst)

	cfg := grpccfg.Config{
		Addr:           "localhost:50051",
		MaxRecvMsgSize: 4 * 1024 * 1024,
		MaxSendMsgSize: 4 * 1024 * 1024,
	}
	cfg.ApplyDefaults()

	// Create a real discovery.Client wrapping the mock
	dcfg := discovery.ClientConfig{CacheTTL: 30}
	dc := discovery.NewClient(mockDisc, dcfg, log)

	factory := NewDiscoveryConnectionFactory(dc, cfg, log)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: This will likely fail because there's no server, but we're
		// benchmarking the discovery and dial option building, not the actual connection.
		factory.NewConn("bench-service")
	}
}
