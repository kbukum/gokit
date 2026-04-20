package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExecutor_SequentialExecution(t *testing.T) {
	t.Parallel()

	steps := []Step[int]{
		{
			ID: "double", Name: "Double",
			Execute: func(_ context.Context, n int) (int, error) { return n * 2, nil },
		},
		{
			ID: "add-ten", Name: "Add Ten",
			Execute: func(_ context.Context, n int) (int, error) { return n + 10, nil },
		},
	}

	exec := NewExecutor(steps)
	result, err := exec.Execute(context.Background(), 5, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 5 * 2 = 10, 10 + 10 = 20
	if result != 20 {
		t.Errorf("expected 20, got %d", result)
	}
}

func TestExecutor_ProgressReporting(t *testing.T) {
	t.Parallel()

	steps := []Step[string]{
		{
			ID: "step-1", Name: "First",
			Execute: func(_ context.Context, s string) (string, error) { return s + "-a", nil },
		},
		{
			ID: "step-2", Name: "Second",
			Execute: func(_ context.Context, s string) (string, error) { return s + "-b", nil },
		},
	}

	var progress []StepProgress[string]
	exec := NewExecutor(steps)
	result, err := exec.Execute(context.Background(), "start", func(p StepProgress[string]) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "start-a-b" {
		t.Errorf("result = %q, want %q", result, "start-a-b")
	}

	// Expect: started, completed, started, completed
	if len(progress) != 4 {
		t.Fatalf("expected 4 progress events, got %d", len(progress))
	}
	expected := []StepStatus{StepStarted, StepCompleted, StepStarted, StepCompleted}
	for i, p := range progress {
		if p.Status != expected[i] {
			t.Errorf("progress[%d].Status = %q, want %q", i, p.Status, expected[i])
		}
	}
}

func TestExecutor_StepFailure(t *testing.T) {
	t.Parallel()

	steps := []Step[int]{
		{
			ID: "ok", Name: "OK",
			Execute: func(_ context.Context, n int) (int, error) { return n + 1, nil },
		},
		{
			ID: "fail", Name: "Fail",
			Execute: func(_ context.Context, n int) (int, error) {
				return 0, fmt.Errorf("boom")
			},
		},
		{
			ID: "never", Name: "Never",
			Execute: func(_ context.Context, n int) (int, error) { return n, nil },
		},
	}

	var progress []StepProgress[int]
	exec := NewExecutor(steps)
	_, err := exec.Execute(context.Background(), 0, func(p StepProgress[int]) {
		progress = append(progress, p)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should contain 'boom', got %q", err.Error())
	}

	// Expect: started(ok), completed(ok), started(fail), failed(fail)
	if len(progress) != 4 {
		t.Fatalf("expected 4 progress events, got %d", len(progress))
	}
	if progress[3].Status != StepFailed {
		t.Errorf("last status = %q, want %q", progress[3].Status, StepFailed)
	}
	if progress[3].Result == nil || progress[3].Result.Err == nil {
		t.Error("expected error in failed step result")
	}
}

func TestExecutor_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	steps := []Step[int]{
		{
			ID: "cancel-step", Name: "Cancel",
			Execute: func(_ context.Context, n int) (int, error) {
				cancel() // cancel context during execution
				return n, nil
			},
		},
		{
			ID: "after-cancel", Name: "After Cancel",
			Execute: func(_ context.Context, n int) (int, error) { return n, nil },
		},
	}

	exec := NewExecutor(steps)
	_, err := exec.Execute(ctx, 1, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error should mention 'canceled', got %q", err.Error())
	}
}

func TestExecutor_SkipStep(t *testing.T) {
	t.Parallel()

	steps := []Step[int]{
		{
			ID: "always", Name: "Always Runs",
			Execute: func(_ context.Context, n int) (int, error) { return n + 1, nil },
		},
		{
			ID: "skipped", Name: "Skipped",
			Execute: func(_ context.Context, n int) (int, error) { return n * 100, nil },
			Skip:    func(_ context.Context, n int) bool { return n > 0 },
		},
		{
			ID: "final", Name: "Final",
			Execute: func(_ context.Context, n int) (int, error) { return n + 5, nil },
		},
	}

	var progress []StepProgress[int]
	exec := NewExecutor(steps)
	result, err := exec.Execute(context.Background(), 0, func(p StepProgress[int]) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 0 + 1 = 1, skip (1 > 0), 1 + 5 = 6
	if result != 6 {
		t.Errorf("expected 6, got %d", result)
	}

	// Find the skipped event.
	var skipped bool
	for _, p := range progress {
		if p.StepID == "skipped" && p.Status == StepSkipped {
			skipped = true
		}
	}
	if !skipped {
		t.Error("expected step 'skipped' to be skipped")
	}
}

func TestExecutor_EmptySteps(t *testing.T) {
	t.Parallel()

	exec := NewExecutor[string](nil)
	result, err := exec.Execute(context.Background(), "unchanged", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "unchanged" {
		t.Errorf("expected 'unchanged', got %q", result)
	}
}

func TestExecutor_ElapsedTime(t *testing.T) {
	t.Parallel()

	steps := []Step[int]{
		{
			ID: "slow", Name: "Slow Step",
			Execute: func(_ context.Context, n int) (int, error) {
				time.Sleep(10 * time.Millisecond)
				return n, nil
			},
		},
	}

	var progress []StepProgress[int]
	exec := NewExecutor(steps)
	_, err := exec.Execute(context.Background(), 0, func(p StepProgress[int]) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Find the completed event.
	for _, p := range progress {
		if p.Status == StepCompleted && p.Result != nil {
			if p.Result.Elapsed < 10*time.Millisecond {
				t.Errorf("elapsed = %v, expected >= 10ms", p.Result.Elapsed)
			}
			return
		}
	}
	t.Error("no completed event with result found")
}

func TestStepResult_Fields(t *testing.T) {
	t.Parallel()

	r := StepResult[string]{
		StepID:  "test",
		Output:  "hello",
		Err:     nil,
		Elapsed: 100 * time.Millisecond,
	}
	if r.StepID != "test" {
		t.Errorf("StepID = %q", r.StepID)
	}
	if r.Output != "hello" {
		t.Errorf("Output = %q", r.Output)
	}
	if r.Elapsed != 100*time.Millisecond {
		t.Errorf("Elapsed = %v", r.Elapsed)
	}
}
