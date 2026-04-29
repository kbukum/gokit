package chain_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/chain"
)

// ── Mock operations ──────────────────────────────────────────────────────

type incrementOp struct {
	chain.BaseOperation
	id string
}

func (o *incrementOp) ID() string   { return o.id }
func (o *incrementOp) Name() string { return o.id }
func (o *incrementOp) Execute(_ context.Context, input any, progress chain.ProgressFn) (any, error) {
	n, _ := input.(int)
	progress(50, "halfway")
	progress(100, "")
	return n + 1, nil
}

type failOp struct {
	chain.BaseOperation
	id string
}

func (o *failOp) ID() string   { return o.id }
func (o *failOp) Name() string { return o.id }
func (o *failOp) Execute(_ context.Context, _ any, _ chain.ProgressFn) (any, error) {
	return nil, fmt.Errorf("intentional failure")
}

type cleanupTracker struct {
	chain.BaseOperation
	id      string
	cleaned *atomic.Bool
}

func (o *cleanupTracker) ID() string   { return o.id }
func (o *cleanupTracker) Name() string { return o.id }
func (o *cleanupTracker) Execute(_ context.Context, input any, _ chain.ProgressFn) (any, error) {
	return input, nil
}

func (o *cleanupTracker) Cleanup(_ context.Context, _ any) error {
	o.cleaned.Store(true)
	return nil
}

// ── Tests ────────────────────────────────────────────────────────────────

func TestSimpleChainIncrements(t *testing.T) {
	c := chain.NewBuilder().
		Step(&incrementOp{id: "step-1"}).
		Step(&incrementOp{id: "step-2"}).
		Step(&incrementOp{id: "step-3"}).
		Build()

	result, err := c.Execute(context.Background(), 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatal("expected success")
	}
	if result.CompletedSteps() != 3 {
		t.Errorf("expected 3 completed steps, got %d", result.CompletedSteps())
	}
	if result.FinalOutput != 3 {
		t.Errorf("expected final output 3, got %v", result.FinalOutput)
	}
	if result.FailedStep() != nil {
		t.Error("expected no failed step")
	}

	for i, step := range result.Steps {
		if step.Status != chain.StatusCompleted {
			t.Errorf("step %d: expected completed, got %s", i, step.Status)
		}
		if step.Output != i+1 {
			t.Errorf("step %d: expected output %d, got %v", i, i+1, step.Output)
		}
	}
}

func TestFailureTriggersCleanup(t *testing.T) {
	cleaned1 := &atomic.Bool{}
	cleaned2 := &atomic.Bool{}

	c := chain.NewBuilder().
		Step(&cleanupTracker{id: "tracker-1", cleaned: cleaned1}).
		Step(&failOp{id: "fail-op"}).
		Step(&cleanupTracker{id: "tracker-2", cleaned: cleaned2}).
		CleanupOnFailure(true).
		StopOnFailure(true).
		Build()

	result, err := c.Execute(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.CompletedSteps() != 1 {
		t.Errorf("expected 1 completed step, got %d", result.CompletedSteps())
	}
	if result.FinalOutput != nil {
		t.Errorf("expected nil final output, got %v", result.FinalOutput)
	}

	// Step 0 completed, step 1 failed, step 2 skipped
	if result.Steps[0].Status != chain.StatusCompleted {
		t.Errorf("step 0: expected completed, got %s", result.Steps[0].Status)
	}
	if result.Steps[1].Status != chain.StatusFailed {
		t.Errorf("step 1: expected failed, got %s", result.Steps[1].Status)
	}
	if result.Steps[2].Status != chain.StatusSkipped {
		t.Errorf("step 2: expected skipped, got %s", result.Steps[2].Status)
	}

	// Cleanup should have run on the completed tracker-1
	if !cleaned1.Load() {
		t.Error("expected tracker-1 to be cleaned up")
	}
	// tracker-2 was skipped, so no cleanup
	if cleaned2.Load() {
		t.Error("expected tracker-2 NOT to be cleaned up")
	}
}

func TestCancellationMarksRemainingCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before the chain runs

	c := chain.NewBuilder().
		Step(&incrementOp{id: "step-1"}).
		Step(&incrementOp{id: "step-2"}).
		Step(&incrementOp{id: "step-3"}).
		Build()

	result, err := c.Execute(ctx, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure due to cancellation")
	}
	if result.CompletedSteps() != 0 {
		t.Errorf("expected 0 completed steps, got %d", result.CompletedSteps())
	}

	for _, step := range result.Steps {
		if step.Status != chain.StatusCanceled {
			t.Errorf("step %s: expected canceled, got %s", step.StepID, step.Status)
		}
		if step.Error != "chain canceled" {
			t.Errorf("step %s: expected error 'chain canceled', got %q", step.StepID, step.Error)
		}
	}
}

func TestContinueAfterFailure(t *testing.T) {
	c := chain.NewBuilder().
		Step(&incrementOp{id: "step-1"}).
		Step(&failOp{id: "fail-op"}).
		Step(&incrementOp{id: "step-3"}).
		StopOnFailure(false).
		CleanupOnFailure(false).
		Build()

	result, err := c.Execute(context.Background(), 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}

	// step-1 completed, fail-op failed, step-3 still ran
	if result.Steps[0].Status != chain.StatusCompleted {
		t.Errorf("step 0: expected completed, got %s", result.Steps[0].Status)
	}
	if result.Steps[1].Status != chain.StatusFailed {
		t.Errorf("step 1: expected failed, got %s", result.Steps[1].Status)
	}
	if result.Steps[2].Status != chain.StatusCompleted {
		t.Errorf("step 2: expected completed, got %s", result.Steps[2].Status)
	}
	if result.CompletedSteps() != 2 {
		t.Errorf("expected 2 completed steps, got %d", result.CompletedSteps())
	}
}

func TestProgressCallbackEvents(t *testing.T) {
	var mu sync.Mutex
	var events []chain.StepProgress

	c := chain.NewBuilder().
		Step(&incrementOp{id: "step-1"}).
		Step(&incrementOp{id: "step-2"}).
		Build()

	progress := func(p chain.StepProgress) {
		mu.Lock()
		events = append(events, p)
		mu.Unlock()
	}

	result, err := c.Execute(context.Background(), 0, progress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}

	mu.Lock()
	captured := make([]chain.StepProgress, len(events))
	copy(captured, events)
	mu.Unlock()

	// For each step: Running(0%), Running(50%), Running(100%), Completed(100%)
	// = 4 events per step × 2 steps = 8 total
	if len(captured) != 8 {
		t.Fatalf("expected 8 progress events, got %d", len(captured))
	}

	// First step events
	assertProgress(t, captured[0], "step-1", chain.StatusRunning, 0)
	assertProgress(t, captured[1], "step-1", chain.StatusRunning, 50)
	assertProgress(t, captured[2], "step-1", chain.StatusRunning, 100)
	assertProgress(t, captured[3], "step-1", chain.StatusCompleted, 100)

	// Second step events
	assertProgress(t, captured[4], "step-2", chain.StatusRunning, 0)
	assertProgress(t, captured[7], "step-2", chain.StatusCompleted, 100)
}

func TestEmptyChain(t *testing.T) {
	c := chain.NewBuilder().Build()

	result, err := c.Execute(context.Background(), "input", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatal("expected success for empty chain")
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(result.Steps))
	}
}

func TestExecutorString(t *testing.T) {
	c := chain.NewBuilder().
		Step(&incrementOp{id: "a"}).
		Step(&incrementOp{id: "b"}).
		Build()

	s := c.String()
	if s == "" {
		t.Error("expected non-empty string representation")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────

func assertProgress(t *testing.T, p chain.StepProgress, stepID string, status chain.StepStatus, pct uint8) {
	t.Helper()
	if p.StepID != stepID {
		t.Errorf("expected step_id %q, got %q", stepID, p.StepID)
	}
	if p.Status != status {
		t.Errorf("step %q: expected status %s, got %s", stepID, status, p.Status)
	}
	if p.ProgressPercent != pct {
		t.Errorf("step %q: expected progress %d%%, got %d%%", stepID, pct, p.ProgressPercent)
	}
}
