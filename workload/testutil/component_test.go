package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
	"github.com/kbukum/gokit/workload"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
	var _ = comp.Manager()
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

func TestMockManager_DeployAndStatus(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	mgr := comp.Manager()

	result, err := mgr.Deploy(ctx, workload.DeployRequest{Name: "test-app", Image: "nginx:latest"})
	if err != nil {
		t.Fatalf("Deploy() failed: %v", err)
	}
	if result.ID == "" {
		t.Error("Deploy() should return non-empty ID")
	}

	status, err := mgr.Status(ctx, result.ID)
	if err != nil {
		t.Fatalf("Status() failed: %v", err)
	}
	if status.Status != workload.StatusRunning {
		t.Errorf("Status = %q, want %q", status.Status, workload.StatusRunning)
	}
	if !status.Running {
		t.Error("Running should be true")
	}
}

func TestMockManager_StopRestartRemove(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	mgr := comp.Manager()
	result, _ := mgr.Deploy(ctx, workload.DeployRequest{Name: "test"})

	// Stop
	mgr.Stop(ctx, result.ID)
	status, _ := mgr.Status(ctx, result.ID)
	if status.Status != workload.StatusStopped {
		t.Errorf("after Stop: Status = %q, want %q", status.Status, workload.StatusStopped)
	}

	// Restart
	mgr.Restart(ctx, result.ID)
	status, _ = mgr.Status(ctx, result.ID)
	if status.Status != workload.StatusRunning {
		t.Errorf("after Restart: Status = %q, want %q", status.Status, workload.StatusRunning)
	}

	// Remove
	mgr.Remove(ctx, result.ID)
	_, err := mgr.Status(ctx, result.ID)
	if err == nil {
		t.Error("Status after Remove should return error")
	}
}

func TestComponent_ResetSnapshotRestore(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	mgr := comp.Manager()
	mgr.Deploy(ctx, workload.DeployRequest{Name: "app-1"})

	snap, _ := comp.Snapshot(ctx)

	mgr.Deploy(ctx, workload.DeployRequest{Name: "app-2"})
	if comp.MockManagerClient().DeployCount() != 2 {
		t.Errorf("DeployCount = %d, want 2", comp.MockManagerClient().DeployCount())
	}

	comp.Restore(ctx, snap)
	if comp.MockManagerClient().DeployCount() != 1 {
		t.Errorf("after Restore: DeployCount = %d, want 1", comp.MockManagerClient().DeployCount())
	}

	comp.Reset(ctx)
	if comp.MockManagerClient().DeployCount() != 0 {
		t.Errorf("after Reset: DeployCount = %d, want 0", comp.MockManagerClient().DeployCount())
	}
}
