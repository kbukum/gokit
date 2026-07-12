package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
)

// mockRegistry implements discovery.Registry for testing.
type mockRegistry struct {
	registered    map[string]*discovery.ServiceInfo
	registerErr   error
	deregisterErr error
	updateErr     error
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		registered: make(map[string]*discovery.ServiceInfo),
	}
}

func (m *mockRegistry) Register(ctx context.Context, service *discovery.ServiceInfo) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered[service.ID] = service
	return nil
}

func (m *mockRegistry) Deregister(ctx context.Context, serviceID string) error {
	if m.deregisterErr != nil {
		return m.deregisterErr
	}
	delete(m.registered, serviceID)
	return nil
}

func (m *mockRegistry) UpdateHealth(ctx context.Context, serviceID string, healthy bool, note string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	return nil
}

func (m *mockRegistry) Stats() discovery.RegistryStats {
	return discovery.RegistryStats{
		RegisteredServices: len(m.registered),
	}
}

func (m *mockRegistry) Close() error {
	return nil
}

// TestDiscoveryServerComponent_LifecycleSuccess tests successful start/stop cycle.
func TestDiscoveryServerComponent_LifecycleSuccess(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()
	cfg := &Config{
		Host: "localhost",
		Port: 9999,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"test-svc-1",
		"test-service",
		"127.0.0.1",
		9999,
		[]string{"test", "v1"},
		map[string]string{"env": "test"},
		log,
	)
	require.NoError(t, err)
	require.NotNil(t, dsc)

	// Start
	ctx := context.Background()
	err = dsc.Start(ctx)
	require.NoError(t, err)

	// Verify registration
	assert.Len(t, registry.registered, 1)
	svcInfo, ok := registry.registered["test-svc-1"]
	assert.True(t, ok)
	assert.Equal(t, "test-service", svcInfo.Name)
	assert.Equal(t, "127.0.0.1", svcInfo.Address)
	assert.Equal(t, 9999, svcInfo.Port)
	assert.Equal(t, []string{"test", "v1"}, svcInfo.Tags)
	assert.Equal(t, map[string]string{"env": "test"}, svcInfo.Metadata)

	// Health check
	health := dsc.Health(ctx)
	assert.Equal(t, component.StatusHealthy, health.Status)

	// Stop
	err = dsc.Stop(ctx)
	require.NoError(t, err)

	// Verify deregistration
	assert.Empty(t, registry.registered)
}

// TestDiscoveryServerComponent_NameAndComponents verifies component properties.
func TestDiscoveryServerComponent_NameAndComponents(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()
	cfg := &Config{
		Host: "localhost",
		Port: 8888,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"svc-123",
		"my-service",
		"192.168.1.1",
		8888,
		nil,
		nil,
		log,
	)
	require.NoError(t, err)

	assert.Equal(t, "discovery-server", dsc.Name())
	assert.NotNil(t, dsc.Server())
	assert.Equal(t, inner, dsc.Server())

	svcInfo := dsc.ServiceInfo()
	assert.Equal(t, "svc-123", svcInfo.ID)
	assert.Equal(t, "my-service", svcInfo.Name)
	assert.Equal(t, "192.168.1.1", svcInfo.Address)
	assert.Equal(t, 8888, svcInfo.Port)
}

// TestDiscoveryServerComponent_RegistrationFailure tests handling of registration errors.
func TestDiscoveryServerComponent_RegistrationFailure(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()
	registry.registerErr = errors.New("registration service unavailable")

	cfg := &Config{
		Host: "localhost",
		Port: 7777,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"test-svc",
		"test-service",
		"127.0.0.1",
		7777,
		nil,
		nil,
		log,
	)
	require.NoError(t, err)

	// Start should fail due to registration error
	ctx := context.Background()
	err = dsc.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to register service")

	// Verify nothing was registered
	assert.Empty(t, registry.registered)
}

// TestDiscoveryServerComponent_DeregistrationError tests that deregistration errors don't prevent shutdown.
func TestDiscoveryServerComponent_DeregistrationError(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()

	cfg := &Config{
		Host: "localhost",
		Port: 6666,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"test-svc",
		"test-service",
		"127.0.0.1",
		6666,
		nil,
		nil,
		log,
	)
	require.NoError(t, err)

	// Start successfully
	ctx := context.Background()
	err = dsc.Start(ctx)
	require.NoError(t, err)
	assert.Len(t, registry.registered, 1)

	// Make deregistration fail
	registry.deregisterErr = errors.New("deregistration failed")

	// Stop should still succeed (but log the error)
	err = dsc.Stop(ctx)
	require.NoError(t, err)

	// Service should be cleaned up from registry regardless
	// (because we explicitly deleted it in the mock)
}

// TestDiscoveryServerComponent_NilRegistry tests validation of required parameters.
func TestDiscoveryServerComponent_NilRegistry(t *testing.T) {
	log := logging.NewDefault("test")
	cfg := &Config{
		Host: "localhost",
		Port: 5555,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		nil, // nil registry
		"test-svc",
		"test-service",
		"127.0.0.1",
		5555,
		nil,
		nil,
		log,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry cannot be nil")
	assert.Nil(t, dsc)
}

// TestDiscoveryServerComponent_NilInnerComponent tests validation of required parameters.
func TestDiscoveryServerComponent_NilInnerComponent(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()

	dsc, err := NewDiscoveryServerComponent(
		nil, // nil inner
		registry,
		"test-svc",
		"test-service",
		"127.0.0.1",
		5555,
		nil,
		nil,
		log,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inner component cannot be nil")
	assert.Nil(t, dsc)
}

// TestDiscoveryServerComponent_LocalIPResolution tests automatic local IP resolution.
func TestDiscoveryServerComponent_LocalIPResolution(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()
	cfg := &Config{
		Host: "localhost",
		Port: 4444,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	// Create with empty address (should fail with current implementation)
	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"test-svc",
		"test-service",
		"", // empty address
		4444,
		nil,
		nil,
		log,
	)
	// Currently expects address to be provided
	require.Error(t, err)
	assert.Nil(t, dsc)
}

// TestDiscoveryServerComponent_Describe tests the Describe method.
func TestDiscoveryServerComponent_Describe(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()
	cfg := &Config{
		Host: "localhost",
		Port: 3333,
	}
	srv := New(cfg, log)
	inner := NewComponent(srv)

	dsc, err := NewDiscoveryServerComponent(
		inner,
		registry,
		"svc-desc",
		"descriptor-service",
		"10.0.0.1",
		3333,
		nil,
		nil,
		log,
	)
	require.NoError(t, err)

	desc := dsc.Describe()
	assert.Equal(t, "Discovery Server", desc.Name)
	assert.Equal(t, "discovery-server", desc.Type)
	assert.Equal(t, 3333, desc.Port)
	assert.Contains(t, desc.Details, "descriptor-service")
}

// TestDiscoveryServerComponent_MultipleInstances tests multiple instances with different IDs.
func TestDiscoveryServerComponent_MultipleInstances(t *testing.T) {
	log := logging.NewDefault("test")
	registry := newMockRegistry()

	// Create first instance
	cfg1 := &Config{Host: "localhost", Port: 2222}
	srv1 := New(cfg1, log)
	inner1 := NewComponent(srv1)
	dsc1, err := NewDiscoveryServerComponent(inner1, registry, "svc-1", "service", "127.0.0.1", 2222, nil, nil, log)
	require.NoError(t, err)

	// Create second instance with different port
	cfg2 := &Config{Host: "localhost", Port: 1111}
	srv2 := New(cfg2, log)
	inner2 := NewComponent(srv2)
	dsc2, err := NewDiscoveryServerComponent(inner2, registry, "svc-2", "service", "127.0.0.1", 1111, nil, nil, log)
	require.NoError(t, err)

	// Both should coexist in registry
	ctx := context.Background()
	err = dsc1.Start(ctx)
	require.NoError(t, err)
	err = dsc2.Start(ctx)
	require.NoError(t, err)

	assert.Len(t, registry.registered, 2)
	assert.Contains(t, registry.registered, "svc-1")
	assert.Contains(t, registry.registered, "svc-2")

	// Stop both
	err = dsc1.Stop(ctx)
	require.NoError(t, err)
	err = dsc2.Stop(ctx)
	require.NoError(t, err)

	assert.Empty(t, registry.registered)
}
