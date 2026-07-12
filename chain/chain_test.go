package chain_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/kbukum/gokit/chain"
	"github.com/kbukum/gokit/errors"
)

func TestExecuteTransformsTypedOutput(t *testing.T) {
	t.Parallel()

	parse := chain.StepFunc("parse", func(_ chain.StepContext, in string) (int, error) {
		return strconv.Atoi(in)
	})
	double := chain.StepFunc("double", func(_ chain.StepContext, n int) (int, error) {
		return n * 2, nil
	})
	format := chain.StepFunc("format", func(_ chain.StepContext, n int) (string, error) {
		return fmt.Sprintf("value=%d", n), nil
	})

	c := chain.Then(chain.Then(chain.Then(chain.New[string](), parse), double), format).Build()

	out, err := c.Execute(context.Background(), "21", nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "value=42" {
		t.Fatalf("got %q, want %q", out, "value=42")
	}
	if c.Len() != 3 {
		t.Fatalf("len = %d, want 3", c.Len())
	}
	if c.IsEmpty() {
		t.Fatal("chain should not be empty")
	}
}

func TestEmptyChainReturnsInput(t *testing.T) {
	t.Parallel()

	c := chain.New[int]().Build()
	if !c.IsEmpty() {
		t.Fatal("empty chain should report IsEmpty")
	}
	out, err := c.Execute(context.Background(), 7, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != 7 {
		t.Fatalf("got %d, want 7", out)
	}
}

func TestStepFailureShortCircuitsAndPreservesCode(t *testing.T) {
	t.Parallel()

	sentinel := errors.NotFound("widget", "42")
	var secondRan bool

	first := chain.StepFunc("first", func(_ chain.StepContext, in int) (int, error) {
		return in, sentinel
	})
	second := chain.StepFunc("second", func(_ chain.StepContext, in int) (int, error) {
		secondRan = true
		return in, nil
	})

	c := chain.Then(chain.Then(chain.New[int](), first), second).Build()
	_, err := c.Execute(context.Background(), 1, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if secondRan {
		t.Fatal("second step must not run after first fails")
	}

	var appErr *errors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("error is not AppError: %v", err)
	}
	if appErr.Code != errors.ErrCodeNotFound {
		t.Fatalf("code = %s, want %s", appErr.Code, errors.ErrCodeNotFound)
	}
	if got := appErr.Details["step"]; got != "first" {
		t.Fatalf("step detail = %v, want first", got)
	}
	// The original sentinel must remain unmutated.
	if _, ok := sentinel.Details["step"]; ok {
		t.Fatal("wrapping mutated the caller's error")
	}
}

func TestCleanupRunsOnFailureInReverseOrder(t *testing.T) {
	t.Parallel()

	var order []string
	var mu sync.Mutex
	record := func(id string) chain.CleanupFn {
		return func(_ context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, id)
			return nil
		}
	}

	a := chain.StepFunc("a", func(_ chain.StepContext, n int) (int, error) { return n, nil }).
		WithCleanup(record("a"))
	b := chain.StepFunc("b", func(_ chain.StepContext, n int) (int, error) { return n, nil }).
		WithCleanup(record("b"))
	fail := chain.StepFunc("fail", func(_ chain.StepContext, n int) (int, error) {
		return 0, errors.Internal(stderrors.New("boom"))
	})

	c := chain.Then(chain.Then(chain.Then(chain.New[int](), a), b), fail).Build()
	if _, err := c.Execute(context.Background(), 1, nil); err == nil {
		t.Fatal("expected failure")
	}

	if len(order) != 2 || order[0] != "b" || order[1] != "a" {
		t.Fatalf("cleanup order = %v, want [b a]", order)
	}
}

func TestCleanupErrorsJoinedOntoFailure(t *testing.T) {
	t.Parallel()

	cleanupErr := stderrors.New("cleanup failed")
	a := chain.StepFunc("a", func(_ chain.StepContext, n int) (int, error) { return n, nil }).
		WithCleanup(func(_ context.Context) error { return cleanupErr })
	fail := chain.StepFunc("fail", func(_ chain.StepContext, n int) (int, error) {
		return 0, errors.Internal(stderrors.New("boom"))
	})

	c := chain.Then(chain.Then(chain.New[int](), a), fail).Build()
	_, err := c.Execute(context.Background(), 1, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !stderrors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error not joined: %v", err)
	}
}

func TestNilStepExecuteReturnsErrorAndRunsCleanups(t *testing.T) {
	t.Parallel()

	cleaned := false
	a := chain.StepFunc("a", func(_ chain.StepContext, n int) (int, error) { return n, nil }).
		WithCleanup(func(_ context.Context) error { cleaned = true; return nil })
	// A zero-value Step has a nil execute function; the runner must surface a
	// normal error instead of panicking.
	var broken chain.Step[int, int]

	c := chain.Then(chain.Then(chain.New[int](), a), broken).Build()
	_, err := c.Execute(context.Background(), 1, nil)
	if err == nil {
		t.Fatal("expected error for nil step execute")
	}
	if !cleaned {
		t.Fatal("expected prior step cleanup to run on nil-execute failure")
	}
}

func TestCancellationBeforeStepRunsCleanups(t *testing.T) {
	t.Parallel()

	var cleaned bool
	a := chain.StepFunc("a", func(_ chain.StepContext, n int) (int, error) { return n, nil }).
		WithCleanup(func(_ context.Context) error { cleaned = true; return nil })

	ctx, cancel := context.WithCancel(context.Background())
	var secondRan bool
	b := chain.StepFunc("b", func(_ chain.StepContext, n int) (int, error) {
		secondRan = true
		return n, nil
	})

	// Cancel after the first step completes but before the second runs.
	trigger := chain.StepFunc("trigger", func(_ chain.StepContext, n int) (int, error) {
		cancel()
		return n, nil
	})

	c := chain.Then(chain.Then(chain.Then(chain.New[int](), a), trigger), b).Build()
	_, err := c.Execute(ctx, 1, nil)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if secondRan {
		t.Fatal("step after cancellation must not run")
	}
	if !cleaned {
		t.Fatal("cleanup should run on cancellation")
	}

	var appErr *errors.AppError
	if !stderrors.As(err, &appErr) || appErr.Code != errors.ErrCodeCanceled {
		t.Fatalf("expected canceled error, got %v", err)
	}
}

func TestProgressCallbackEmitsRunningThenCompleted(t *testing.T) {
	t.Parallel()

	step := chain.StepFunc("work", func(sctx chain.StepContext, n int) (int, error) {
		sctx.Progress(200, "clamped") // over 100 must clamp
		return n, nil
	})

	var updates []chain.StepProgress
	c := chain.Then(chain.New[int](), step).Build()
	if _, err := c.Execute(context.Background(), 1, func(p chain.StepProgress) {
		updates = append(updates, p)
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(updates) != 3 {
		t.Fatalf("got %d updates, want 3: %+v", len(updates), updates)
	}
	if updates[0].Status != chain.StatusRunning || updates[0].ProgressPercent != 0 {
		t.Fatalf("first update = %+v", updates[0])
	}
	if updates[1].Status != chain.StatusRunning || updates[1].ProgressPercent != 100 || updates[1].Message != "clamped" {
		t.Fatalf("step-local update = %+v", updates[1])
	}
	if updates[2].Status != chain.StatusCompleted || updates[2].ProgressPercent != 100 {
		t.Fatalf("final update = %+v", updates[2])
	}
	if updates[2].StepID != "work" || updates[2].StepIndex != 0 {
		t.Fatalf("final update metadata = %+v", updates[2])
	}
}

func TestStepAccessors(t *testing.T) {
	t.Parallel()

	s := chain.NewStep("id", "display", func(_ chain.StepContext, n int) (int, error) { return n, nil })
	if s.ID() != "id" || s.Name() != "display" {
		t.Fatalf("accessors = %q/%q", s.ID(), s.Name())
	}
	f := chain.StepFunc("only", func(_ chain.StepContext, n int) (int, error) { return n, nil })
	if f.ID() != "only" || f.Name() != "only" {
		t.Fatalf("StepFunc accessors = %q/%q", f.ID(), f.Name())
	}
}

func TestStepContextErrReflectsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	step := chain.StepFunc("check", func(sctx chain.StepContext, n int) (int, error) {
		if sctx.Err() != nil {
			t.Error("context should be live inside step")
		}
		if sctx.Context() == nil {
			t.Error("Context() must not be nil")
		}
		return n, nil
	})
	defer cancel()

	c := chain.Then(chain.New[int](), step).Build()
	if _, err := c.Execute(ctx, 1, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
}
