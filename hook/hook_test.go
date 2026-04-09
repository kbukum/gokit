package hook_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

func TestEventTypes(t *testing.T) {
	events := []struct {
		event    hook.Event
		expected hook.EventType
	}{
		{hook.PreToolCall{Name: "test"}, hook.EventPreToolCall},
		{hook.PostToolCall{Name: "test"}, hook.EventPostToolCall},
		{hook.PreLLMCall{}, hook.EventPreLLMCall},
		{hook.PostLLMCall{}, hook.EventPostLLMCall},
		{hook.OnError{Err: fmt.Errorf("err"), Source: "test"}, hook.EventOnError},
		{hook.TurnStart{Turn: 1}, hook.EventTurnStart},
		{hook.TurnEnd{Turn: 1}, hook.EventTurnEnd},
	}

	for _, tc := range events {
		t.Run(string(tc.expected), func(t *testing.T) {
			if got := tc.event.Type(); got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestPreToolCall_Fields(t *testing.T) {
	input := json.RawMessage(`{"key": "value"}`)
	e := hook.PreToolCall{Name: "calculator", Input: input}

	if e.Name != "calculator" {
		t.Errorf("Name = %q, want %q", e.Name, "calculator")
	}
	if string(e.Input) != `{"key": "value"}` {
		t.Errorf("Input = %s, want %s", e.Input, input)
	}
}

func TestPostToolCall_Fields(t *testing.T) {
	result := tool.TextResult("42")
	e := hook.PostToolCall{
		Name:   "calculator",
		Input:  json.RawMessage(`{}`),
		Result: result,
		Err:    nil,
	}

	if e.Name != "calculator" {
		t.Errorf("Name = %q, want %q", e.Name, "calculator")
	}
	if e.Result != result {
		t.Error("Result mismatch")
	}
	if e.Err != nil {
		t.Error("Err should be nil")
	}
}

func TestPreLLMCall_Fields(t *testing.T) {
	temp := 0.7
	req := llm.CompletionRequest{
		Model:       "gpt-4",
		MaxTokens:   1024,
		Temperature: &temp,
	}
	e := hook.PreLLMCall{Request: req}

	if e.Request.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", e.Request.Model, "gpt-4")
	}
}

func TestTurnEvents(t *testing.T) {
	start := hook.TurnStart{Turn: 3}
	if start.Turn != 3 {
		t.Errorf("TurnStart.Turn = %d, want 3", start.Turn)
	}

	end := hook.TurnEnd{
		Turn:    3,
		Message: llm.Assistant("done"),
	}
	if end.Turn != 3 {
		t.Errorf("TurnEnd.Turn = %d, want 3", end.Turn)
	}
}

// --- Result constructors ---

func TestContinue(t *testing.T) {
	r := hook.Continue()
	if r.Action != hook.ActionContinue {
		t.Errorf("Action = %v, want Continue", r.Action)
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
	result := reg.Emit(hook.TurnStart{Turn: 1})
	if result.Action != hook.ActionContinue {
		t.Errorf("expected Continue with no handlers, got %v", result.Action)
	}
}

func TestRegistry_SingleHandler(t *testing.T) {
	reg := hook.NewRegistry()
	var received hook.EventType

	reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
		received = e.Type()
		return hook.Continue()
	})

	reg.Emit(hook.TurnStart{Turn: 1})

	if received != hook.EventTurnStart {
		t.Errorf("handler received %q, want %q", received, hook.EventTurnStart)
	}
}

func TestRegistry_MultipleHandlers_ExecutionOrder(t *testing.T) {
	reg := hook.NewRegistry()
	var order []int

	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		order = append(order, 1)
		return hook.Continue()
	})
	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		order = append(order, 2)
		return hook.Continue()
	})
	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		order = append(order, 3)
		return hook.Continue()
	})

	reg.Emit(hook.PreToolCall{Name: "test"})

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected order [1,2,3], got %v", order)
	}
}

func TestRegistry_AbortShortCircuits(t *testing.T) {
	reg := hook.NewRegistry()
	handlersCalled := 0

	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		handlersCalled++
		return hook.Abort("blocked")
	})
	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		handlersCalled++
		return hook.Continue()
	})

	result := reg.Emit(hook.PreToolCall{Name: "dangerous_tool"})

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

	reg.On(hook.EventPreLLMCall, func(e hook.Event) hook.Result {
		return hook.Modify("step1")
	})
	reg.On(hook.EventPreLLMCall, func(e hook.Event) hook.Result {
		return hook.Modify("step2")
	})

	result := reg.Emit(hook.PreLLMCall{})

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

	unsub := reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
		calls++
		return hook.Continue()
	})

	reg.Emit(hook.TurnStart{Turn: 1})
	if calls != 1 {
		t.Fatalf("expected 1 call before unsub, got %d", calls)
	}

	unsub()

	reg.Emit(hook.TurnStart{Turn: 2})
	if calls != 1 {
		t.Errorf("expected 1 call after unsub, got %d", calls)
	}
}

func TestRegistry_DifferentEventTypes(t *testing.T) {
	reg := hook.NewRegistry()
	var toolCalls, turnCalls int

	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		toolCalls++
		return hook.Continue()
	})
	reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
		turnCalls++
		return hook.Continue()
	})

	reg.Emit(hook.PreToolCall{Name: "test"})
	reg.Emit(hook.PreToolCall{Name: "test2"})
	reg.Emit(hook.TurnStart{Turn: 1})

	if toolCalls != 2 {
		t.Errorf("toolCalls = %d, want 2", toolCalls)
	}
	if turnCalls != 1 {
		t.Errorf("turnCalls = %d, want 1", turnCalls)
	}
}

func TestRegistry_HasHandlers(t *testing.T) {
	reg := hook.NewRegistry()

	if reg.HasHandlers(hook.EventTurnStart) {
		t.Error("should have no handlers initially")
	}

	unsub := reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
		return hook.Continue()
	})

	if !reg.HasHandlers(hook.EventTurnStart) {
		t.Error("should have handlers after On")
	}

	unsub()

	if reg.HasHandlers(hook.EventTurnStart) {
		t.Error("should have no handlers after unsub")
	}
}

func TestRegistry_Clear(t *testing.T) {
	reg := hook.NewRegistry()

	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result { return hook.Continue() })
	reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result { return hook.Continue() })

	reg.Clear(hook.EventPreToolCall)
	if reg.HasHandlers(hook.EventPreToolCall) {
		t.Error("should have cleared PreToolCall")
	}
	if !reg.HasHandlers(hook.EventTurnStart) {
		t.Error("TurnStart should still have handlers")
	}
}

func TestRegistry_ClearAll(t *testing.T) {
	reg := hook.NewRegistry()

	reg.On(hook.EventPreToolCall, func(e hook.Event) hook.Result { return hook.Continue() })
	reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result { return hook.Continue() })

	reg.Clear()
	if reg.HasHandlers(hook.EventPreToolCall) || reg.HasHandlers(hook.EventTurnStart) {
		t.Error("all handlers should be cleared")
	}
}

func TestRegistry_ConcurrentSafety(t *testing.T) {
	reg := hook.NewRegistry()
	done := make(chan struct{})

	// Register handlers concurrently
	for i := range 10 {
		go func(n int) {
			reg.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
				return hook.Continue()
			})
			done <- struct{}{}
		}(i)
	}

	// Emit events concurrently
	for range 10 {
		go func() {
			reg.Emit(hook.TurnStart{Turn: 1})
			done <- struct{}{}
		}()
	}

	for range 20 {
		<-done
	}
}

func TestOnError_Fields(t *testing.T) {
	err := fmt.Errorf("connection refused")
	e := hook.OnError{Err: err, Source: "llm_provider"}

	if e.Err.Error() != "connection refused" {
		t.Errorf("Err = %v, want connection refused", e.Err)
	}
	if e.Source != "llm_provider" {
		t.Errorf("Source = %q, want llm_provider", e.Source)
	}
}

func TestPostLLMCall_WithError(t *testing.T) {
	e := hook.PostLLMCall{
		Err: fmt.Errorf("timeout"),
	}

	if e.Err == nil || e.Err.Error() != "timeout" {
		t.Errorf("expected timeout error, got %v", e.Err)
	}
}
