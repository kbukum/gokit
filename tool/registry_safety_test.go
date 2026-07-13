package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kbukum/gokit/tool"
)

type gatedInput struct {
	Query string `json:"query" jsonschema:"required,description=Search text"`
}

func newGatedTool(t *testing.T, called *bool) *tool.Registry {
	t.Helper()
	tl := tool.FromFuncInputOnly("search", "Search", func(_ context.Context, in gatedInput) (string, error) {
		*called = true
		return in.Query, nil
	})
	reg := tool.NewRegistry()
	if err := reg.Register(tl.AsCallable()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg
}

func TestRegistry_Call_RejectsInvalidInput(t *testing.T) {
	called := false
	reg := newGatedTool(t, &called)

	// Missing the required "query" field must fail closed before invocation.
	_, err := reg.Call(tool.NewContext(context.Background()), "search", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for schema-invalid input")
	}
	if !errors.Is(err, tool.ErrInvalidToolInput) {
		t.Fatalf("error = %v, want ErrInvalidToolInput", err)
	}
	if called {
		t.Fatal("tool handler must not run on invalid input")
	}
}

func TestRegistry_Call_RejectsMalformedJSON(t *testing.T) {
	called := false
	reg := newGatedTool(t, &called)

	_, err := reg.Call(tool.NewContext(context.Background()), "search", json.RawMessage(`{"query":`))
	if err == nil || !errors.Is(err, tool.ErrInvalidToolInput) {
		t.Fatalf("error = %v, want ErrInvalidToolInput", err)
	}
	if called {
		t.Fatal("tool handler must not run on malformed input")
	}
}

func TestRegistry_Call_AcceptsValidInput(t *testing.T) {
	called := false
	reg := newGatedTool(t, &called)

	res, err := reg.Call(tool.NewContext(context.Background()), "search", json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !called || res == nil {
		t.Fatal("valid input should invoke the tool")
	}
}

func destructiveRegistry(t *testing.T, called *bool) *tool.Registry {
	t.Helper()
	tl := tool.FromFuncInputOnly("wipe", "Delete everything", func(_ context.Context, in gatedInput) (string, error) {
		*called = true
		return "done", nil
	})
	tl.Def.Envelope.Safety = tool.SafetyDestructive
	reg := tool.NewRegistry()
	if err := reg.Register(tl.AsCallable()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg
}

func TestRegistry_Call_DestructiveDeniedByDefault(t *testing.T) {
	called := false
	reg := destructiveRegistry(t, &called)

	_, err := reg.Call(tool.NewContext(context.Background()), "wipe", json.RawMessage(`{"query":"x"}`))
	if err == nil || !errors.Is(err, tool.ErrToolDenied) {
		t.Fatalf("error = %v, want ErrToolDenied", err)
	}
	if called {
		t.Fatal("destructive tool must not run without human approval")
	}
}

type approveAll struct{ approved bool }

func (a *approveAll) Approve(context.Context, tool.ToolCall) (bool, error) {
	a.approved = true
	return true, nil
}

type countingApprover struct{ calls int }

func (c *countingApprover) Approve(context.Context, tool.ToolCall) (bool, error) {
	c.calls++
	return true, nil
}

type requireApprovalEvaluator struct{}

func (requireApprovalEvaluator) Evaluate(context.Context, tool.ToolCall, tool.SensitivePredicate) (tool.Decision, string, error) {
	return tool.DecisionRequireApproval, "sensitive", nil
}

// A destructive tool that also carries a sensitive predicate resolving to
// DecisionRequireApproval must elicit human approval exactly once per dispatch,
// not once for the sensitivity path and again for the destructive gate.
func TestRegistry_Call_DestructiveAndSensitiveApprovesOnce(t *testing.T) {
	called := false
	tl := tool.FromFuncInputOnly("wipe", "Delete everything", func(_ context.Context, in gatedInput) (string, error) {
		called = true
		return in.Query, nil
	})
	tl.Def.Envelope.Safety = tool.SafetyDestructive
	tl.Def.Envelope.SensitiveInvocations = []tool.SensitivePredicate{{JSONPath: "$.query", Matcher: tool.MatcherExists}}
	reg := tool.NewRegistry()
	if err := reg.Register(tl.AsCallable()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	approver := &countingApprover{}
	reg.WithSensitivityEvaluator(requireApprovalEvaluator{}).WithHumanApproval(approver)

	res, err := reg.Call(tool.NewContext(context.Background()), "wipe", json.RawMessage(`{"query":"x"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !called || res == nil {
		t.Fatal("approved destructive tool should run")
	}
	if approver.calls != 1 {
		t.Fatalf("human approver called %d times, want exactly 1", approver.calls)
	}
}

func TestRegistry_Call_DestructiveProceedsWithApproval(t *testing.T) {
	called := false
	reg := destructiveRegistry(t, &called)
	approver := &approveAll{}
	reg.WithHumanApproval(approver)

	res, err := reg.Call(tool.NewContext(context.Background()), "wipe", json.RawMessage(`{"query":"x"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !approver.approved {
		t.Fatal("destructive tool must route through the human approver")
	}
	if !called || res == nil {
		t.Fatal("approved destructive tool should run")
	}
}
