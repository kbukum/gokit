package hook_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/hook"
)

// --- Test event types (domain-agnostic) ---

const (
	eventAlpha hook.EventType = "alpha"
	eventBeta  hook.EventType = "beta"
	eventGamma hook.EventType = "gamma"
)

type alphaEvent struct {
	Value string
}

func (alphaEvent) Type() hook.EventType { return eventAlpha }

type betaEvent struct {
	Count int
}

func (betaEvent) Type() hook.EventType { return eventBeta }

type gammaEvent struct {
	Err    error
	Source string
}

func (gammaEvent) Type() hook.EventType { return eventGamma }

// --- Event interface ---

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

// --- Result constructors ---

func TestContinue(t *testing.T) {
	r := hook.Continue()
	if r.Action != hook.ActionContinue {
		t.Errorf("Action = %v, want Continue", r.Action)
	}
}

func TestContinueWithError(t *testing.T) {
	err := errors.New("something went wrong")
	r := hook.ContinueWithError(err)
	if r.Action != hook.ActionContinue {
		t.Errorf("Action = %v, want Continue", r.Action)
	}
	if !errors.Is(r.Err, err) {
		t.Errorf("Err = %v, want %v", r.Err, err)
	}
}

func TestAbort(t *testing.T) {
	r := hook.Abort("rate limited")
	if r.Action != hook.ActionAbort {
		t.Errorf("Action = %v, want Abort", r.Action)
	}
	if r.Reason != "rate limited" {
		t.Errorf("Reason = %q, want %q", r.Reason, "rate limited")
	}
}

func TestAbortWithError(t *testing.T) {
	err := errors.New("forbidden")
	r := hook.AbortWithError("rate limited", err)
	if r.Action != hook.ActionAbort {
		t.Errorf("Action = %v, want Abort", r.Action)
	}
	if r.Reason != "rate limited" {
		t.Errorf("Reason = %q, want %q", r.Reason, "rate limited")
	}
	if !errors.Is(r.Err, err) {
		t.Errorf("Err = %v, want %v", r.Err, err)
	}
}

func TestModify(t *testing.T) {
	data := map[string]string{"model": "gpt-3.5"}
	r := hook.Modify(data)
	if r.Action != hook.ActionModify {
		t.Errorf("Action = %v, want Modify", r.Action)
	}
	if r.ModifiedData == nil {
		t.Error("ModifiedData should not be nil")
	}
}

// --- Registry tests ---

func TestRegistry_EmitWithNoHandlers(t *testing.T) {
	reg := hook.NewRegistry()
	result := reg.Emit(context.Background(), alphaEvent{Value: "test"})
	if result.Action != hook.ActionContinue {
		t.Errorf("expected Continue with no handlers, got %v", result.Action)
	}
}

func TestRegistry_SingleHandler(t *testing.T) {
	reg := hook.NewRegistry()
	var received hook.EventType

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		received = e.Type()
		return hook.Continue()
	})

	reg.Emit(context.Background(), alphaEvent{Value: "hello"})

	if received != eventAlpha {
		t.Errorf("handler received %q, want %q", received, eventAlpha)
	}
}

func TestRegistry_TypeAssertionInHandler(t *testing.T) {
	reg := hook.NewRegistry()
	var capturedValue string

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		a := e.(alphaEvent)
		capturedValue = a.Value
		return hook.Continue()
	})

	reg.Emit(context.Background(), alphaEvent{Value: "captured!"})

	if capturedValue != "captured!" {
		t.Errorf("capturedValue = %q, want %q", capturedValue, "captured!")
	}
}

func TestRegistry_MultipleHandlers_ExecutionOrder(t *testing.T) {
	reg := hook.NewRegistry()
	var order []int

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		order = append(order, 1)
		return hook.Continue()
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		order = append(order, 2)
		return hook.Continue()
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		order = append(order, 3)
		return hook.Continue()
	})

	reg.Emit(context.Background(), alphaEvent{Value: "test"})

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
}

func TestRegistry_AbortShortCircuits(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		handlersCalled++
		return hook.Abort("blocked")
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		handlersCalled++
		return hook.Continue()
	})

	result := reg.Emit(context.Background(), alphaEvent{Value: "dangerous"})

	if result.Action != hook.ActionAbort {
		t.Errorf("expected Abort, got %v", result.Action)
	}
	if result.Reason != "blocked" {
		t.Errorf("Reason = %q, want %q", result.Reason, "blocked")
	}
	if handlersCalled != 1 {
		t.Errorf("expected 1 handler called (short-circuit), got %d", handlersCalled)
	}
}

func TestRegistry_ModifyChains(t *testing.T) {
	reg := hook.NewRegistry()

	reg.On(eventBeta, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.Modify("step1")
	})
	reg.On(eventBeta, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.Modify("step2")
	})

	result := reg.Emit(context.Background(), betaEvent{Count: 1})

	if result.Action != hook.ActionModify {
		t.Errorf("expected Modify, got %v", result.Action)
	}
	if result.ModifiedData != "step2" {
		t.Errorf("ModifiedData = %v, want step2", result.ModifiedData)
	}
}

func TestRegistry_Unsubscribe(t *testing.T) {
	reg := hook.NewRegistry()
	calls := 0

	unsub := reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		calls++
		return hook.Continue()
	})

	reg.Emit(context.Background(), alphaEvent{Value: "1"})
	if calls != 1 {
		t.Fatalf("expected 1 call before unsub, got %d", calls)
	}

	unsub()

	reg.Emit(context.Background(), alphaEvent{Value: "2"})
	if calls != 1 {
		t.Errorf("expected 1 call after unsub, got %d", calls)
	}
}

func TestRegistry_DifferentEventTypes(t *testing.T) {
	reg := hook.NewRegistry()
	var alphaCalls, betaCalls int

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		alphaCalls++
		return hook.Continue()
	})
	reg.On(eventBeta, func(ctx context.Context, e hook.Event) hook.Result {
		betaCalls++
		return hook.Continue()
	})

	ctx := context.Background()
	reg.Emit(ctx, alphaEvent{Value: "a"})
	reg.Emit(ctx, alphaEvent{Value: "b"})
	reg.Emit(ctx, betaEvent{Count: 1})

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

	unsub := reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.Continue()
	})

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

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result { return hook.Continue() })
	reg.On(eventBeta, func(ctx context.Context, e hook.Event) hook.Result { return hook.Continue() })

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

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result { return hook.Continue() })
	reg.On(eventBeta, func(ctx context.Context, e hook.Event) hook.Result { return hook.Continue() })

	reg.Clear()
	if reg.HasHandlers(eventAlpha) || reg.HasHandlers(eventBeta) {
		t.Error("all handlers should be cleared")
	}
}

func TestRegistry_ConcurrentSafety(t *testing.T) {
	reg := hook.NewRegistry()
	done := make(chan struct{})

	for i := range 10 {
		go func(n int) {
			reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
				return hook.Continue()
			})
			done <- struct{}{}
		}(i)
	}

	for range 10 {
		go func() {
			reg.Emit(context.Background(), alphaEvent{Value: "concurrent"})
			done <- struct{}{}
		}()
	}

	for range 20 {
		<-done
	}
}

// --- Context support tests ---

func TestRegistry_ContextCancellation(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		handlersCalled++
		return hook.Continue()
	})

	result := reg.Emit(ctx, alphaEvent{Value: "test"})

	if result.Action != hook.ActionAbort {
		t.Errorf("expected Abort on canceled context, got %v", result.Action)
	}
	if result.Err == nil {
		t.Error("expected error on canceled context")
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
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		received = ctx.Value(ctxKey{}).(string)
		return hook.Continue()
	})

	reg.Emit(ctx, alphaEvent{Value: "test"})

	if received != "injected" {
		t.Errorf("handler received ctx value %q, want %q", received, "injected")
	}
}

// --- Panic recovery tests ---

func TestRegistry_PanicRecovery(t *testing.T) {
	reg := hook.NewRegistry()
	var order []int

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		order = append(order, 1)
		return hook.Continue()
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		panic("handler exploded")
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		order = append(order, 3)
		return hook.Continue()
	})

	result := reg.Emit(context.Background(), alphaEvent{Value: "test"})

	if len(order) != 2 || order[0] != 1 || order[1] != 3 {
		t.Errorf("expected order [1,3] (skipping panicked handler), got %v", order)
	}
	if result.Err == nil {
		t.Error("expected error from panicked handler")
	}
	if !strings.Contains(result.Err.Error(), "panicked") {
		t.Errorf("error should mention panic, got: %v", result.Err)
	}
}

func TestRegistry_PanicDoesNotPreventAbort(t *testing.T) {
	reg := hook.NewRegistry()

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		panic("boom")
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.Abort("stop")
	})

	result := reg.Emit(context.Background(), alphaEvent{Value: "test"})

	if result.Action != hook.ActionAbort {
		t.Errorf("expected Abort after panic recovery, got %v", result.Action)
	}
}

// --- Error propagation tests ---

func TestRegistry_ContinueWithErrorPropagates(t *testing.T) {
	reg := hook.NewRegistry()
	expectedErr := errors.New("non-fatal issue")

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.ContinueWithError(expectedErr)
	})

	result := reg.Emit(context.Background(), alphaEvent{Value: "test"})

	if result.Action != hook.ActionContinue {
		t.Errorf("expected Continue, got %v", result.Action)
	}
	if !errors.Is(result.Err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, result.Err)
	}
}

func TestRegistry_LastContinueErrorWins(t *testing.T) {
	reg := hook.NewRegistry()
	err1 := errors.New("first")
	err2 := errors.New("second")

	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.ContinueWithError(err1)
	})
	reg.On(eventAlpha, func(ctx context.Context, e hook.Event) hook.Result {
		return hook.ContinueWithError(err2)
	})

	result := reg.Emit(context.Background(), alphaEvent{Value: "test"})

	if !errors.Is(result.Err, err2) {
		t.Errorf("expected last error %v, got %v", err2, result.Err)
	}
}
