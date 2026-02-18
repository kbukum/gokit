package testutil_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/testutil"
)

// TestHelpers_Setup tests the Setup helper function
func TestHelpers_Setup(t *testing.T) {
	comp := newMockComponent("test")

	cleanup, err := testutil.Setup(comp)
	if err != nil {
		t.Fatalf("Setup() failed: %v", err)
	}

	if !comp.started {
		t.Error("component should be started after Setup()")
	}

	// Cleanup should stop the component
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() failed: %v", err)
	}

	if !comp.stopped {
		t.Error("component should be stopped after cleanup()")
	}
}

// TestHelpers_SetupWithContext tests Setup with custom context
func TestHelpers_SetupWithContext(t *testing.T) {
	ctx := context.Background()
	comp := newMockComponent("test")

	cleanup, err := testutil.SetupWithContext(ctx, comp)
	if err != nil {
		t.Fatalf("SetupWithContext() failed: %v", err)
	}

	if !comp.started {
		t.Error("component should be started after SetupWithContext()")
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() failed: %v", err)
	}

	if !comp.stopped {
		t.Error("component should be stopped after cleanup()")
	}
}

// TestHelpers_Teardown tests the Teardown helper function
func TestHelpers_Teardown(t *testing.T) {
	comp := newMockComponent("test")

	// Start component first
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Teardown should stop it
	if err := testutil.Teardown(comp); err != nil {
		t.Fatalf("Teardown() failed: %v", err)
	}

	if !comp.stopped {
		t.Error("component should be stopped after Teardown()")
	}
}

// TestHelpers_TeardownWithContext tests Teardown with custom context
func TestHelpers_TeardownWithContext(t *testing.T) {
	ctx := context.Background()
	comp := newMockComponent("test")

	// Start component first
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Teardown should stop it
	if err := testutil.TeardownWithContext(ctx, comp); err != nil {
		t.Fatalf("TeardownWithContext() failed: %v", err)
	}

	if !comp.stopped {
		t.Error("component should be stopped after TeardownWithContext()")
	}
}

// TestHelpers_ResetComponent tests the Reset helper
func TestHelpers_ResetComponent(t *testing.T) {
	comp := newMockComponent("test")

	if err := testutil.ResetComponent(comp); err != nil {
		t.Fatalf("ResetComponent() failed: %v", err)
	}

	if !comp.resetCalled {
		t.Error("Reset() should have been called")
	}
}

// TestHelpers_ResetComponentWithContext tests Reset with custom context
func TestHelpers_ResetComponentWithContext(t *testing.T) {
	ctx := context.Background()
	comp := newMockComponent("test")

	if err := testutil.ResetComponentWithContext(ctx, comp); err != nil {
		t.Fatalf("ResetComponentWithContext() failed: %v", err)
	}

	if !comp.resetCalled {
		t.Error("Reset() should have been called")
	}
}

// TestHelpers_T_Setup tests the T helper for testing.T integration
func TestHelpers_T_Setup(t *testing.T) {
	comp := newMockComponent("test")

	// T.Setup should register cleanup automatically
	testutil.T(t).Setup(comp)

	if !comp.started {
		t.Error("component should be started after T.Setup()")
	}

	// Component will be cleaned up automatically by testing.T.Cleanup
}

// TestHelpers_T_Reset tests the T helper Reset functionality
func TestHelpers_T_Reset(t *testing.T) {
	comp := newMockComponent("test")

	testutil.T(t).Reset(comp)

	if !comp.resetCalled {
		t.Error("Reset() should have been called")
	}
}

// TestHelpers_MultipleComponents tests managing multiple components
func TestHelpers_MultipleComponents(t *testing.T) {
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")

	cleanup1, err := testutil.Setup(comp1)
	if err != nil {
		t.Fatalf("Setup(comp1) failed: %v", err)
	}

	cleanup2, err := testutil.Setup(comp2)
	if err != nil {
		t.Fatalf("Setup(comp2) failed: %v", err)
	}

	if !comp1.started || !comp2.started {
		t.Error("both components should be started")
	}

	// Cleanup in reverse order (LIFO)
	if err := cleanup2(); err != nil {
		t.Fatalf("cleanup2() failed: %v", err)
	}
	if err := cleanup1(); err != nil {
		t.Fatalf("cleanup1() failed: %v", err)
	}

	if !comp1.stopped || !comp2.stopped {
		t.Error("both components should be stopped")
	}
}
