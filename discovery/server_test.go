package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/component"
)

type fakeInner struct {
	started, stopped bool
	startErr         error
	stopErr          error
	healthy          bool
}

func (f *fakeInner) Name() string { return "inner" }
func (f *fakeInner) Start(context.Context) error {
	f.started = true
	return f.startErr
}

func (f *fakeInner) Stop(context.Context) error {
	f.stopped = true
	return f.stopErr
}

func (f *fakeInner) Health(context.Context) component.Health {
	if f.healthy {
		return component.Health{Name: "inner", Status: component.StatusHealthy}
	}
	return component.Health{Name: "inner", Status: component.StatusUnhealthy, Message: "down"}
}

type fakeRegistry struct {
	registered   []string
	deregistered []string
	registerErr  error
	deregErr     error
}

func (r *fakeRegistry) Register(_ context.Context, svc *ServiceInfo) error {
	if r.registerErr != nil {
		return r.registerErr
	}
	r.registered = append(r.registered, svc.ID)
	return nil
}

func (r *fakeRegistry) Deregister(_ context.Context, id string) error {
	if r.deregErr != nil {
		return r.deregErr
	}
	r.deregistered = append(r.deregistered, id)
	return nil
}
func (r *fakeRegistry) UpdateHealth(context.Context, string, bool, string) error { return nil }
func (r *fakeRegistry) Stats() RegistryStats                                     { return RegistryStats{} }
func (r *fakeRegistry) Close() error                                             { return nil }

func testService() *ServiceInfo {
	return &ServiceInfo{ID: "svc-1", Name: "svc", Address: "127.0.0.1", Port: 8080}
}

func TestNewDiscoveryServer_Validation(t *testing.T) {
	t.Parallel()
	reg := &fakeRegistry{}
	if _, err := NewDiscoveryServer("ds", nil, reg, testService(), nil); err == nil {
		t.Error("expected error for nil inner")
	}
	if _, err := NewDiscoveryServer("ds", &fakeInner{}, nil, testService(), nil); err == nil {
		t.Error("expected error for nil registry")
	}
	if _, err := NewDiscoveryServer("ds", &fakeInner{}, reg, nil, nil); err == nil {
		t.Error("expected error for nil service")
	}
}

func TestDiscoveryServer_LifecycleSuccess(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{healthy: true}
	reg := &fakeRegistry{}
	ds, err := NewDiscoveryServer("ds", inner, reg, testService(), nil)
	if err != nil {
		t.Fatalf("NewDiscoveryServer: %v", err)
	}
	ctx := context.Background()

	if err := ds.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !inner.started {
		t.Error("inner not started")
	}
	if len(reg.registered) != 1 || reg.registered[0] != "svc-1" {
		t.Errorf("registered = %v, want [svc-1]", reg.registered)
	}
	if h := ds.Health(ctx); h.Status != component.StatusHealthy {
		t.Errorf("health = %v, want healthy", h.Status)
	}

	if err := ds.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !inner.stopped {
		t.Error("inner not stopped")
	}
	if len(reg.deregistered) != 1 || reg.deregistered[0] != "svc-1" {
		t.Errorf("deregistered = %v, want [svc-1]", reg.deregistered)
	}
}

func TestDiscoveryServer_RegistrationFailureStopsInner(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{}
	reg := &fakeRegistry{registerErr: errors.New("registry down")}
	ds, _ := NewDiscoveryServer("ds", inner, reg, testService(), nil)

	err := ds.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start error on registration failure")
	}
	if !inner.started {
		t.Error("inner should have started before registration")
	}
	if !inner.stopped {
		t.Error("inner should be stopped after registration failure")
	}
}

func TestDiscoveryServer_InnerStartFailureSkipsRegistration(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{startErr: errors.New("boom")}
	reg := &fakeRegistry{}
	ds, _ := NewDiscoveryServer("ds", inner, reg, testService(), nil)

	if err := ds.Start(context.Background()); err == nil {
		t.Fatal("expected Start error")
	}
	if len(reg.registered) != 0 {
		t.Errorf("registered = %v, want none", reg.registered)
	}
}

func TestDiscoveryServer_DeregisterFailureStillStops(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{healthy: true}
	reg := &fakeRegistry{deregErr: errors.New("dereg failed")}
	ds, _ := NewDiscoveryServer("ds", inner, reg, testService(), nil)
	ctx := context.Background()
	if err := ds.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := ds.Stop(ctx); err != nil {
		t.Fatalf("Stop should succeed despite deregister failure: %v", err)
	}
	if !inner.stopped {
		t.Error("inner should be stopped")
	}
}

func TestDiscoveryServer_HealthReflectsInner(t *testing.T) {
	t.Parallel()
	inner := &fakeInner{healthy: false}
	ds, _ := NewDiscoveryServer("ds", inner, &fakeRegistry{}, testService(), nil)
	h := ds.Health(context.Background())
	if h.Status != component.StatusUnhealthy {
		t.Errorf("health = %v, want unhealthy", h.Status)
	}
}
