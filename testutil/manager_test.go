package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/testutil"
)

// TestManager_NewManager tests manager creation
func TestManager_NewManager(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	if manager == nil {
		t.Fatal("NewManager() should return non-nil manager")
	}
}

// TestManager_AddComponent tests adding components
func TestManager_AddComponent(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	// Components should be tracked
	components := manager.Components()
	if len(components) != 2 {
		t.Errorf("Components() = %d, want 2", len(components))
	}
}

// TestManager_StartAll tests starting all components
func TestManager_StartAll(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	if err := manager.StartAll(); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}
	
	if !comp1.started {
		t.Error("comp1 should be started")
	}
	if !comp2.started {
		t.Error("comp2 should be started")
	}
}

// TestManager_StopAll tests stopping all components
func TestManager_StopAll(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	// Start first
	if err := manager.StartAll(); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}
	
	// Then stop
	if err := manager.StopAll(); err != nil {
		t.Fatalf("StopAll() failed: %v", err)
	}
	
	if !comp1.stopped {
		t.Error("comp1 should be stopped")
	}
	if !comp2.stopped {
		t.Error("comp2 should be stopped")
	}
}

// TestManager_ResetAll tests resetting all components
func TestManager_ResetAll(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	if err := manager.ResetAll(); err != nil {
		t.Fatalf("ResetAll() failed: %v", err)
	}
	
	if !comp1.resetCalled {
		t.Error("comp1.Reset() should be called")
	}
	if !comp2.resetCalled {
		t.Error("comp2.Reset() should be called")
	}
}

// TestManager_StartError tests error handling during StartAll
func TestManager_StartError(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	comp2.startErr = errors.New("start failed")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	err := manager.StartAll()
	if err == nil {
		t.Error("StartAll() should return error when component fails")
	}
}

// TestManager_StopError tests error handling during StopAll
func TestManager_StopError(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	comp2.stopErr = errors.New("stop failed")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	// Start components
	if err := manager.StartAll(); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}
	
	// Stop should collect errors
	err := manager.StopAll()
	if err == nil {
		t.Error("StopAll() should return error when component fails")
	}
}

// TestManager_ResetError tests error handling during ResetAll
func TestManager_ResetError(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	comp2.resetErr = errors.New("reset failed")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	err := manager.ResetAll()
	if err == nil {
		t.Error("ResetAll() should return error when component fails")
	}
}

// TestManager_GetComponent tests retrieving components by name
func TestManager_GetComponent(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	
	manager.Add(comp1)
	manager.Add(comp2)
	
	// Get existing component
	got := manager.Get("comp1")
	if got == nil {
		t.Error("Get('comp1') should return component")
	}
	if got != nil && got.Name() != "comp1" {
		t.Errorf("Get('comp1').Name() = %q, want 'comp1'", got.Name())
	}
	
	// Get non-existing component
	got = manager.Get("nonexistent")
	if got != nil {
		t.Error("Get('nonexistent') should return nil")
	}
}

// TestManager_Cleanup tests the Cleanup method (convenience wrapper for StopAll)
func TestManager_Cleanup(t *testing.T) {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)
	
	comp := newMockComponent("comp")
	manager.Add(comp)
	
	if err := manager.StartAll(); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}
	
	if err := manager.Cleanup(); err != nil {
		t.Fatalf("Cleanup() failed: %v", err)
	}
	
	if !comp.stopped {
		t.Error("component should be stopped after Cleanup()")
	}
}
