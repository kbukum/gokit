package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

// mockComponent is a test implementation of TestComponent
type mockComponent struct {
	name            string
	started         bool
	stopped         bool
	resetCalled     bool
	snapshotData    interface{}
	restoreData     interface{}
	startErr        error
	stopErr         error
	resetErr        error
	snapshotErr     error
	restoreErr      error
	healthStatus    component.HealthStatus
	healthMessage   string
}

func newMockComponent(name string) *mockComponent {
	return &mockComponent{
		name:          name,
		healthStatus:  component.StatusHealthy,
		healthMessage: "OK",
		snapshotData:  map[string]interface{}{name + "_key": name + "_value"},
	}
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Start(ctx context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	m.stopped = false
	return nil
}

func (m *mockComponent) Stop(ctx context.Context) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	m.started = false
	return nil
}

func (m *mockComponent) Health(ctx context.Context) component.Health {
	return component.Health{
		Name:    m.name,
		Status:  m.healthStatus,
		Message: m.healthMessage,
	}
}

func (m *mockComponent) Reset(ctx context.Context) error {
	if m.resetErr != nil {
		return m.resetErr
	}
	m.resetCalled = true
	m.snapshotData = map[string]interface{}{m.name + "_key": m.name + "_value"}
	return nil
}

func (m *mockComponent) Snapshot(ctx context.Context) (interface{}, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.snapshotData, nil
}

func (m *mockComponent) Restore(ctx context.Context, snapshot interface{}) error {
	if m.restoreErr != nil {
		return m.restoreErr
	}
	m.restoreData = snapshot
	return nil
}

// TestComponent_Interface verifies TestComponent extends component.Component
func TestComponent_Interface(t *testing.T) {
	mock := newMockComponent("test")
	
	// Should be assignable to component.Component
	var _ component.Component = mock
	
	// Should be assignable to testutil.TestComponent
	var _ testutil.TestComponent = mock
}

// TestComponent_BasicLifecycle tests basic Start/Stop/Health
func TestComponent_BasicLifecycle(t *testing.T) {
	ctx := context.Background()
	mock := newMockComponent("test")

	// Initial state
	if mock.started {
		t.Error("component should not be started initially")
	}

	// Start
	if err := mock.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if !mock.started {
		t.Error("component should be started after Start()")
	}

	// Health check
	health := mock.Health(ctx)
	if health.Name != "test" {
		t.Errorf("Health().Name = %q, want %q", health.Name, "test")
	}
	if health.Status != component.StatusHealthy {
		t.Errorf("Health().Status = %q, want %q", health.Status, component.StatusHealthy)
	}

	// Stop
	if err := mock.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	if !mock.stopped {
		t.Error("component should be stopped after Stop()")
	}
}

// TestComponent_Reset tests the Reset method
func TestComponent_Reset(t *testing.T) {
	ctx := context.Background()
	mock := newMockComponent("test")

	if err := mock.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}
	if !mock.resetCalled {
		t.Error("Reset() should have been called")
	}
}

// TestComponent_SnapshotRestore tests Snapshot and Restore
func TestComponent_SnapshotRestore(t *testing.T) {
	ctx := context.Background()
	mock := newMockComponent("test")

	// Take snapshot
	snapshot, err := mock.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}
	if snapshot == nil {
		t.Error("Snapshot() should return non-nil data")
	}

	// Restore from snapshot
	if err := mock.Restore(ctx, snapshot); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}
	if mock.restoreData == nil {
		t.Error("Restore() should have set restore data")
	}
}

// TestComponent_ErrorHandling tests error scenarios
func TestComponent_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		setupErr  func(*mockComponent)
		operation func(*mockComponent) error
	}{
		{
			name:     "Start error",
			setupErr: func(m *mockComponent) { m.startErr = errors.New("start failed") },
			operation: func(m *mockComponent) error {
				return m.Start(ctx)
			},
		},
		{
			name:     "Stop error",
			setupErr: func(m *mockComponent) { m.stopErr = errors.New("stop failed") },
			operation: func(m *mockComponent) error {
				return m.Stop(ctx)
			},
		},
		{
			name:     "Reset error",
			setupErr: func(m *mockComponent) { m.resetErr = errors.New("reset failed") },
			operation: func(m *mockComponent) error {
				return m.Reset(ctx)
			},
		},
		{
			name:     "Snapshot error",
			setupErr: func(m *mockComponent) { m.snapshotErr = errors.New("snapshot failed") },
			operation: func(m *mockComponent) error {
				_, err := m.Snapshot(ctx)
				return err
			},
		},
		{
			name:     "Restore error",
			setupErr: func(m *mockComponent) { m.restoreErr = errors.New("restore failed") },
			operation: func(m *mockComponent) error {
				return m.Restore(ctx, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockComponent("test")
			tt.setupErr(mock)
			
			err := tt.operation(mock)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
