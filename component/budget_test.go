package component_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
)

// budgetTolerance absorbs scheduling between taking the shutdown deadline and a component reading its own.
const budgetTolerance = 150 * time.Millisecond

type drainer struct {
	componenttest.Component
	phase component.DrainPhase
	drain func(context.Context) error
}

func (d *drainer) Drain(ctx context.Context) error  { return d.drain(ctx) }
func (d *drainer) DrainPhase() component.DrainPhase { return d.phase }

func startAll(t *testing.T, r *component.Registry, cs ...component.Component) {
	t.Helper()
	for _, c := range cs {
		if err := r.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.StartAll(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func remaining(ctx context.Context) time.Duration {
	deadline, _ := ctx.Deadline()
	return time.Until(deadline)
}

func assertNear(t *testing.T, label string, got, want time.Duration) {
	t.Helper()
	if got < want-budgetTolerance || got > want+budgetTolerance {
		t.Errorf("%s = %s, want %s ± %s", label, got, want, budgetTolerance)
	}
}

func TestDrainBudgetDoesNotShrinkWithComponentCount(t *testing.T) {
	t.Parallel()
	const total = 2 * time.Second
	for _, others := range []int{0, 10} {
		t.Run(fmt.Sprintf("%d other components", others), func(t *testing.T) {
			t.Parallel()
			var got time.Duration
			r := component.NewRegistry()
			cs := make([]component.Component, 0, 1+others)
			cs = append(cs, &drainer{Component: componenttest.Component{ComponentName: "listener"}, phase: component.DrainIngress, drain: func(ctx context.Context) error {
				got = remaining(ctx)
				return nil
			}})
			for i := range others {
				cs = append(cs, &componenttest.Component{ComponentName: fmt.Sprintf("resource-%d", i)})
			}
			startAll(t, r, cs...)
			ctx, cancel := context.WithTimeout(t.Context(), total)
			defer cancel()
			if err := r.Shutdown(ctx, nil); err != nil {
				t.Fatal(err)
			}
			assertNear(t, "ingress drain budget", got, total-total/component.StopReserveDivisor)
		})
	}
}

func TestDrainersInOnePhaseRunConcurrently(t *testing.T) {
	t.Parallel()
	var started sync.WaitGroup
	started.Add(2)
	both := make(chan struct{})
	go func() { started.Wait(); close(both) }()
	wait := func(ctx context.Context) error {
		started.Done()
		select {
		case <-both:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("drained alone: %w", ctx.Err())
		}
	}
	r := component.NewRegistry()
	startAll(t, r,
		&drainer{Component: componenttest.Component{ComponentName: "public"}, phase: component.DrainIngress, drain: wait},
		&drainer{Component: componenttest.Component{ComponentName: "internal"}, phase: component.DrainIngress, drain: wait},
	)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx, nil); err != nil {
		t.Fatal(err)
	}
}

func TestIngressDrainsBeforeWorkersAndLeavesThemItsUnusedTime(t *testing.T) {
	t.Parallel()
	const total = 2 * time.Second
	var (
		mu      sync.Mutex
		order   []string
		workers time.Duration
	)
	record := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, name)
	}
	r := component.NewRegistry()
	startAll(t, r,
		&drainer{Component: componenttest.Component{ComponentName: "worker"}, phase: component.DrainWorkers, drain: func(ctx context.Context) error {
			workers = remaining(ctx)
			record("worker")
			return nil
		}},
		&drainer{Component: componenttest.Component{ComponentName: "listener"}, phase: component.DrainIngress, drain: func(context.Context) error {
			record("listener")
			return nil
		}},
	)
	ctx, cancel := context.WithTimeout(t.Context(), total)
	defer cancel()
	if err := r.Shutdown(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "listener" || order[1] != "worker" {
		t.Fatalf("drain order = %v, want listener then worker", order)
	}
	assertNear(t, "worker drain budget", workers, total-total/component.StopReserveDivisor)
}

func TestStopsKeepTheirReserveAfterADrainUsesItsWholeWindow(t *testing.T) {
	t.Parallel()
	const total = 2 * time.Second
	var stopBudget time.Duration
	r := component.NewRegistry()
	startAll(t, r,
		&componenttest.Component{ComponentName: "database", StopFunc: func(ctx context.Context) error {
			stopBudget = remaining(ctx)
			return nil
		}},
		&drainer{Component: componenttest.Component{ComponentName: "listener"}, phase: component.DrainIngress, drain: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}},
	)
	ctx, cancel := context.WithTimeout(t.Context(), total)
	defer cancel()
	if err := r.Shutdown(ctx, nil); err != nil {
		t.Fatal(err)
	}
	// Two stops (listener, database) share the reserve; the listener stops first and returns at once.
	if floor := total / component.StopReserveDivisor / 2; stopBudget < floor-budgetTolerance {
		t.Fatalf("database stop budget = %s, want at least %s", stopBudget, floor)
	}
}
