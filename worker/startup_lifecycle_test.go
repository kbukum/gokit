package worker

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/config"
)

func TestPoolFailedStartupOwnsNoGoroutines(t *testing.T) {
	for _, configureFailure := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			app, err := bootstrap.NewApp(&config.ServiceConfig{Name: "failed-start"})
			if err != nil {
				t.Fatal(err)
			}
			app.Summary.SetWriter(io.Discard)
			pool := NewPool(HandlerFunc[int, int](func(context.Context, int, func(Event[int])) error { return nil }), PoolConfig{Size: 1})
			cause := errors.New("startup failure")
			if !configureFailure {
				if err := app.RegisterComponent(&testutil.Component{ComponentName: "dependency", StartFunc: func(context.Context) error { return cause }}); err != nil {
					t.Fatal(err)
				}
			}
			if err := app.RegisterComponent(pool); err != nil {
				t.Fatal(err)
			}
			if configureFailure {
				app.OnConfigure(func(context.Context, *bootstrap.App[*config.ServiceConfig]) error { return cause })
			}
			if err := app.Startup(t.Context()); !errors.Is(err, cause) {
				t.Fatalf("startup: %v", err)
			}
			done := make(chan struct{})
			go func() { pool.wg.Wait(); pool.supWg.Wait(); close(done) }()
			synctest.Wait()
			select {
			case <-done:
			default:
				t.Error("unstarted registered pool retained goroutines after failed startup")
				if err := pool.Stop(t.Context()); err != nil {
					t.Fatal(err)
				}
				<-done
			}
		})
	}
}

func TestResultOnlyTaskHasOneTerminalEvent(t *testing.T) {
	t.Parallel()
	pool := NewPool(HandlerFunc[int, int](func(_ context.Context, task int, emit func(Event[int])) error {
		emit(Event[int]{Type: EventResult, Data: task})
		return nil
	}), PoolConfig{Size: 1, EventBuffer: 1})
	defer func() { _ = pool.Stop(context.Background()) }()
	handle, err := pool.Submit(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := handle.Result(); result != 42 || err != nil {
		t.Fatalf("result %d, %v", result, err)
	}
	var events []Event[int]
	for event := range handle.Events() {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != EventResult || events[0].Data != 42 {
		t.Fatalf("terminal delivery: %+v", events)
	}
}
