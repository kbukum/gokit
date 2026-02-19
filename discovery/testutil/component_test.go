package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
	var _ discovery.Registry = comp
	var _ discovery.Discovery = comp
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health = %q, want %q", health.Status, component.StatusHealthy)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponent_DiscoverAndRegister(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	comp.AddInstance("api", discovery.ServiceInstance{
		ID: "api-1", Name: "api", Address: "10.0.0.1", Port: 8080,
		Health: discovery.HealthHealthy,
	})
	comp.AddInstance("api", discovery.ServiceInstance{
		ID: "api-2", Name: "api", Address: "10.0.0.2", Port: 8080,
		Health: discovery.HealthHealthy,
	})

	comp.Start(ctx)
	defer comp.Stop(ctx)

	instances, err := comp.Discover(ctx, "api")
	if err != nil {
		t.Fatalf("Discover() failed: %v", err)
	}
	if len(instances) != 2 {
		t.Errorf("Discover() = %d instances, want 2", len(instances))
	}

	// Unknown service
	_, err = comp.Discover(ctx, "unknown")
	if err != discovery.ErrServiceNotFound {
		t.Errorf("Discover(unknown) = %v, want ErrServiceNotFound", err)
	}

	// Register a service
	comp.Register(ctx, &discovery.ServiceInfo{ID: "svc-1", Name: "test-svc"})
	stats := comp.Stats()
	if stats.RegisteredServices != 1 {
		t.Errorf("Stats.RegisteredServices = %d, want 1", stats.RegisteredServices)
	}

	comp.Deregister(ctx, "svc-1")
	stats = comp.Stats()
	if stats.RegisteredServices != 0 {
		t.Errorf("Stats.RegisteredServices = %d, want 0", stats.RegisteredServices)
	}
}

func TestComponent_ResetSnapshotRestore(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	comp.AddInstance("svc", discovery.ServiceInstance{
		ID: "svc-1", Name: "svc", Address: "10.0.0.1", Port: 80,
		Health: discovery.HealthHealthy,
	})

	comp.Start(ctx)
	defer comp.Stop(ctx)

	snap, err := comp.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	// Modify
	comp.AddInstance("svc", discovery.ServiceInstance{
		ID: "svc-2", Name: "svc", Address: "10.0.0.2", Port: 80,
		Health: discovery.HealthHealthy,
	})

	// Restore
	if err := comp.Restore(ctx, snap); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	instances, _ := comp.Discover(ctx, "svc")
	if len(instances) != 1 {
		t.Errorf("After Restore: %d instances, want 1", len(instances))
	}

	// Reset
	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}
	_, err = comp.Discover(ctx, "svc")
	if err != discovery.ErrServiceNotFound {
		t.Errorf("After Reset: Discover() = %v, want ErrServiceNotFound", err)
	}
}

func TestComponent_HealthFilter(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	comp.AddInstance("svc", discovery.ServiceInstance{
		ID: "healthy-1", Name: "svc", Address: "10.0.0.1", Port: 80,
		Health: discovery.HealthHealthy,
	})
	comp.AddInstance("svc", discovery.ServiceInstance{
		ID: "unhealthy-1", Name: "svc", Address: "10.0.0.2", Port: 80,
		Health: discovery.HealthUnhealthy,
	})

	comp.Start(ctx)
	defer comp.Stop(ctx)

	instances, err := comp.Discover(ctx, "svc")
	if err != nil {
		t.Fatalf("Discover() failed: %v", err)
	}
	if len(instances) != 1 {
		t.Errorf("Discover() = %d healthy instances, want 1", len(instances))
	}
	if instances[0].ID != "healthy-1" {
		t.Errorf("Instance ID = %q, want %q", instances[0].ID, "healthy-1")
	}
}
