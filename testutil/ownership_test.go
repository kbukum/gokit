package testutil_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	componenttest "github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/testutil"
)

func TestManagerRollsBackWithFreshCleanupBudget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	startErr, stopErr := errors.New("start"), errors.New("stop")
	var order []string
	manager := testutil.NewManager(ctx)
	for _, name := range []string{"first", "second"} {
		manager.Add(&componenttest.Component{
			ComponentName: name,
			StopFunc: func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Errorf("cleanup inherited cancellation: %v", ctx.Err())
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("cleanup has no deadline")
				}
				order = append(order, name)
				if name == "first" {
					return stopErr
				}
				return nil
			},
		})
	}
	manager.Add(&componenttest.Component{ComponentName: "failed", StartFunc: func(context.Context) error {
		cancel()
		return startErr
	}})
	err := manager.StartAll()
	if !errors.Is(err, startErr) || !errors.Is(err, stopErr) {
		t.Fatalf("startup did not preserve both errors: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"second", "first"}) {
		t.Fatalf("rollback order: %v", order)
	}
	if err := manager.Cleanup(); !errors.Is(err, stopErr) {
		t.Fatalf("repeated cleanup lost the recorded failure: %v", err)
	}
	if len(order) != 2 {
		t.Fatal("cleanup repeated a completed stop")
	}
}

func TestSetupCancellationAndRepeatedCleanup(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	stops := 0
	cleanup, err := testutil.SetupWithContext(ctx, &componenttest.Component{
		ComponentName: "owned",
		StopFunc: func(ctx context.Context) error {
			stops++
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for range 2 {
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
	}
	if stops != 1 {
		t.Fatalf("stop calls = %d", stops)
	}
}

func TestLifecycleSetupAndCleanupBounds(t *testing.T) {
	t.Parallel()
	budgets := testutil.Budgets{Setup: 100 * time.Millisecond, Cleanup: 100 * time.Millisecond}
	comp := &componenttest.Component{ComponentName: "deadline", StartFunc: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	manager := testutil.NewManager(t.Context(), testutil.WithBudgets(budgets))
	manager.Add(comp)
	if err := manager.StartAll(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("setup deadline: %v", err)
	}
	comp.StartFunc = nil
	comp.StopFunc = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := manager.StartAll(); err != nil {
		t.Fatal(err)
	}
	if err := manager.StopAll(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cleanup deadline: %v", err)
	}
}

func TestConcurrentCleanupAndOwnedRestart(t *testing.T) {
	t.Parallel()
	var starts, stops int
	comp := &componenttest.Component{
		ComponentName: "owned",
		StartFunc:     func(context.Context) error { starts++; return nil },
		StopFunc:      func(context.Context) error { stops++; return nil },
	}
	manager := testutil.NewManager(t.Context())
	if err := manager.Add(comp); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.StartAll(); err != nil {
			t.Fatal(err)
		}
		if err := manager.StartAll(); err == nil {
			t.Fatal("accepted double startup")
		}
		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				if err := manager.Cleanup(); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	}
	if starts != 2 || stops != 2 {
		t.Fatalf("lifecycle counts = %d/%d", starts, stops)
	}
}
