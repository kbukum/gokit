package hook_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/hook"
)

const (
	eventAlpha hook.EventType = "alpha"
	eventBeta  hook.EventType = "beta"
	eventGamma hook.EventType = "gamma"
)

type alphaEvent struct{ Value string }

func (alphaEvent) Type() hook.EventType { return eventAlpha }

type betaEvent struct{ Count int }

func (betaEvent) Type() hook.EventType { return eventBeta }

type gammaEvent struct {
	Err    error
	Source string
}

func (gammaEvent) Type() hook.EventType { return eventGamma }

func TestEventInterface(t *testing.T) {
	events := []struct {
		event    hook.Event
		expected hook.EventType
	}{
		{alphaEvent{Value: "test"}, eventAlpha},
		{betaEvent{Count: 42}, eventBeta},
		{gammaEvent{Source: "test"}, eventGamma},
	}

	for _, tc := range events {
		t.Run(string(tc.expected), func(t *testing.T) {
			if got := tc.event.Type(); got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestErrFatalHookSentinel(t *testing.T) {
	err := errors.Join(errors.New("boom"), hook.ErrFatalHook)
	if !errors.Is(err, hook.ErrFatalHook) {
		t.Fatalf("expected fatal sentinel match")
	}
}

func TestRegistry_EmitWithNoHandlers(t *testing.T) {
	reg := hook.NewRegistry()
	if err := reg.Emit(context.Background(), alphaEvent{Value: "test"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
}

func TestRegistry_SingleHandler(t *testing.T) {
	reg := hook.NewRegistry()
	var received hook.EventType

	reg.On(eventAlpha, func(_ context.Context, e hook.Event) error {
		received = e.Type()
		return nil
	})

	if err := reg.Emit(context.Background(), alphaEvent{Value: "hello"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	if received != eventAlpha {
		t.Errorf("handler received %q, want %q", received, eventAlpha)
	}
}

func TestRegistry_TypeAssertionInHandler(t *testing.T) {
	reg := hook.NewRegistry()
	var capturedValue string

	reg.On(eventAlpha, func(_ context.Context, e hook.Event) error {
		a := e.(alphaEvent)
		capturedValue = a.Value
		return nil
	})

	if err := reg.Emit(context.Background(), alphaEvent{Value: "captured!"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	if capturedValue != "captured!" {
		t.Errorf("capturedValue = %q, want %q", capturedValue, "captured!")
	}
}

func TestRegistry_MultipleHandlers_ExecutionOrder(t *testing.T) {
	reg := hook.NewRegistry()
	var order []int

	reg.On(eventAlpha, func(context.Context, hook.Event) error { order = append(order, 1); return nil })
	reg.On(eventAlpha, func(context.Context, hook.Event) error { order = append(order, 2); return nil })
	reg.On(eventAlpha, func(context.Context, hook.Event) error { order = append(order, 3); return nil })

	if err := reg.Emit(context.Background(), alphaEvent{Value: "test"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
}

func TestRegistry_NonFatalErrorDoesNotShortCircuit(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0
	expectedErr := errors.New("observe failed")

	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return expectedErr
	})
	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return nil
	})

	err := reg.Emit(context.Background(), alphaEvent{Value: "observed"})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if handlersCalled != 2 {
		t.Errorf("expected 2 handlers called, got %d", handlersCalled)
	}
}

func TestRegistry_FatalErrorShortCircuits(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0
	errFatal := errors.Join(errors.New("stop"), hook.ErrFatalHook)

	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return errFatal
	})
	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return nil
	})

	err := reg.Emit(context.Background(), alphaEvent{Value: "dangerous"})
	if !errors.Is(err, hook.ErrFatalHook) {
		t.Fatalf("expected fatal error, got %v", err)
	}
	if handlersCalled != 1 {
		t.Errorf("expected 1 handler called, got %d", handlersCalled)
	}
}

func TestRegistry_EmitsOnErrorForEachNonFatalError(t *testing.T) {
	reg := hook.NewRegistry()
	err1 := errors.New("first")
	err2 := errors.New("second")
	var observed []hook.ErrorEvent

	reg.On(hook.EventOnError, func(_ context.Context, e hook.Event) error {
		observed = append(observed, e.(hook.ErrorEvent))
		return nil
	})
	reg.On(eventAlpha, func(context.Context, hook.Event) error { return err1 })
	reg.On(eventAlpha, func(context.Context, hook.Event) error { return err2 })

	err := reg.Emit(context.Background(), alphaEvent{Value: "test"})
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Fatalf("expected joined errors, got %v", err)
	}
	if len(observed) != 2 {
		t.Fatalf("observed %d errors, want 2", len(observed))
	}
	if observed[0].Source != eventAlpha || !errors.Is(observed[0].Err, err1) {
		t.Fatalf("first observed = %+v", observed[0])
	}
	if observed[1].Source != eventAlpha || !errors.Is(observed[1].Err, err2) {
		t.Fatalf("second observed = %+v", observed[1])
	}
}

func TestRegistry_ErrorEventHandlerErrorAggregatesWithoutRecursion(t *testing.T) {
	reg := hook.NewRegistry()
	primary := errors.New("primary")
	errorHandlerErr := errors.New("error handler")
	calls := 0

	reg.On(hook.EventOnError, func(context.Context, hook.Event) error {
		calls++
		return errorHandlerErr
	})
	reg.On(eventAlpha, func(context.Context, hook.Event) error { return primary })

	err := reg.Emit(context.Background(), alphaEvent{Value: "test"})
	if !errors.Is(err, primary) || !errors.Is(err, errorHandlerErr) {
		t.Fatalf("expected both errors, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("on_error calls = %d, want 1", calls)
	}
}

func TestRegistry_FatalOnErrorShortCircuitsDispatch(t *testing.T) {
	reg := hook.NewRegistry()
	primary := errors.New("primary")
	fatal := errors.Join(errors.New("fatal on_error"), hook.ErrFatalHook)
	handlersCalled := 0

	reg.On(hook.EventOnError, func(context.Context, hook.Event) error { return fatal })
	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return primary
	})
	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return nil
	})

	err := reg.Emit(context.Background(), alphaEvent{Value: "test"})
	if !errors.Is(err, primary) || !errors.Is(err, hook.ErrFatalHook) {
		t.Fatalf("expected primary + fatal errors, got %v", err)
	}
	if handlersCalled != 1 {
		t.Fatalf("handlersCalled = %d, want 1", handlersCalled)
	}
}

func TestRegistry_Unsubscribe(t *testing.T) {
	reg := hook.NewRegistry()
	calls := 0

	unsub := reg.On(eventAlpha, func(context.Context, hook.Event) error {
		calls++
		return nil
	})

	_ = reg.Emit(context.Background(), alphaEvent{Value: "1"})
	if calls != 1 {
		t.Fatalf("expected 1 call before unsub, got %d", calls)
	}

	unsub()
	_ = reg.Emit(context.Background(), alphaEvent{Value: "2"})
	if calls != 1 {
		t.Errorf("expected 1 call after unsub, got %d", calls)
	}
}

func TestRegistry_DifferentEventTypes(t *testing.T) {
	reg := hook.NewRegistry()
	var alphaCalls, betaCalls int

	reg.On(eventAlpha, func(context.Context, hook.Event) error { alphaCalls++; return nil })
	reg.On(eventBeta, func(context.Context, hook.Event) error { betaCalls++; return nil })

	ctx := context.Background()
	_ = reg.Emit(ctx, alphaEvent{Value: "a"})
	_ = reg.Emit(ctx, alphaEvent{Value: "b"})
	_ = reg.Emit(ctx, betaEvent{Count: 1})

	if alphaCalls != 2 {
		t.Errorf("alphaCalls = %d, want 2", alphaCalls)
	}
	if betaCalls != 1 {
		t.Errorf("betaCalls = %d, want 1", betaCalls)
	}
}

func TestRegistry_HasHandlers(t *testing.T) {
	reg := hook.NewRegistry()
	if reg.HasHandlers(eventAlpha) {
		t.Error("should have no handlers initially")
	}
	unsub := reg.On(eventAlpha, func(context.Context, hook.Event) error { return nil })
	if !reg.HasHandlers(eventAlpha) {
		t.Error("should have handlers after On")
	}
	unsub()
	if reg.HasHandlers(eventAlpha) {
		t.Error("should have no handlers after unsub")
	}
}

func TestRegistry_Clear(t *testing.T) {
	reg := hook.NewRegistry()
	reg.On(eventAlpha, func(context.Context, hook.Event) error { return nil })
	reg.On(eventBeta, func(context.Context, hook.Event) error { return nil })

	reg.Clear(eventAlpha)
	if reg.HasHandlers(eventAlpha) {
		t.Error("should have cleared alpha")
	}
	if !reg.HasHandlers(eventBeta) {
		t.Error("beta should still have handlers")
	}
}

func TestRegistry_ClearAll(t *testing.T) {
	reg := hook.NewRegistry()
	reg.On(eventAlpha, func(context.Context, hook.Event) error { return nil })
	reg.On(eventBeta, func(context.Context, hook.Event) error { return nil })

	reg.Clear()
	if reg.HasHandlers(eventAlpha) || reg.HasHandlers(eventBeta) {
		t.Error("all handlers should be cleared")
	}
}

func TestRegistry_ConcurrentSafety(t *testing.T) {
	reg := hook.NewRegistry()
	done := make(chan struct{})

	for range 10 {
		go func() {
			reg.On(eventAlpha, func(context.Context, hook.Event) error { return nil })
			done <- struct{}{}
		}()
	}
	for range 10 {
		go func() {
			_ = reg.Emit(context.Background(), alphaEvent{Value: "concurrent"})
			done <- struct{}{}
		}()
	}
	for range 20 {
		<-done
	}
}

func TestRegistry_ContextCancellation(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reg.On(eventAlpha, func(context.Context, hook.Event) error {
		handlersCalled++
		return nil
	})

	err := reg.Emit(ctx, alphaEvent{Value: "test"})
	if !errors.Is(err, hook.ErrFatalHook) || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected fatal context error, got %v", err)
	}
	if handlersCalled != 0 {
		t.Errorf("expected 0 handlers called on canceled context, got %d", handlersCalled)
	}
}

func TestRegistry_ContextPassedToHandler(t *testing.T) {
	reg := hook.NewRegistry()
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "injected")

	var received string
	reg.On(eventAlpha, func(ctx context.Context, _ hook.Event) error {
		received = ctx.Value(ctxKey{}).(string)
		return nil
	})

	if err := reg.Emit(ctx, alphaEvent{Value: "test"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	if received != "injected" {
		t.Errorf("handler received ctx value %q, want %q", received, "injected")
	}
}

func TestRegistry_PanicRecovery(t *testing.T) {
	reg := hook.NewRegistry()
	var order []int

	reg.On(eventAlpha, func(context.Context, hook.Event) error { order = append(order, 1); return nil })
	reg.On(eventAlpha, func(context.Context, hook.Event) error { panic("handler exploded") })
	reg.On(eventAlpha, func(context.Context, hook.Event) error { order = append(order, 3); return nil })

	err := reg.Emit(context.Background(), alphaEvent{Value: "test"})
	if len(order) != 2 || order[0] != 1 || order[1] != 3 {
		t.Errorf("expected order [1,3], got %v", order)
	}
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should mention panic, got: %v", err)
	}
}
