package tool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

func mkSensitiveTool(t *testing.T, name string, predicates []tool.SensitivePredicate) tool.Callable {
	t.Helper()
	def := tool.Definition{
		Name:        name,
		Description: "test",
		InputSchema: schema.JSON{"type": "object"},
		Envelope:    tool.Envelope{SensitiveInvocations: predicates},
	}
	tt := tool.NewTool(def, tool.HandlerFunc[map[string]any, map[string]any](
		func(_ context.Context, in map[string]any) (map[string]any, error) {
			return map[string]any{"echo": in}, nil
		}))
	return tt.AsCallable()
}

func TestRegistry_Call_NoSensitivity_Allows(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry()
	if err := r.Register(mkSensitiveTool(t, "plain", nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := r.Call(tool.Background(), "plain", []byte(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("res = %+v", res)
	}
}

func TestRegistry_Call_DenyOnSensitive_DefaultDenies(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.amount", Matcher: tool.MatcherGT, Value: "1000"}}
	r := tool.NewRegistry()
	if err := r.Register(mkSensitiveTool(t, "wire", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "wire", []byte(`{"amount":2000}`))
	if !errors.Is(err, tool.ErrToolDenied) {
		t.Fatalf("want ErrToolDenied, got %v", err)
	}
}

type approveAlways struct{}

func (approveAlways) Evaluate(context.Context, tool.ToolCall, tool.SensitivePredicate) (tool.Decision, string, error) {
	return tool.DecisionRequireApproval, "needs review", nil
}

type humanYes struct{ called bool }

func (h *humanYes) Approve(context.Context, tool.ToolCall) (bool, error) {
	h.called = true
	return true, nil
}

type humanNo struct{}

func (humanNo) Approve(context.Context, tool.ToolCall) (bool, error) { return false, nil }

func TestRegistry_Call_RequireApproval_Human_Approves(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.x", Matcher: tool.MatcherExists}}
	h := &humanYes{}
	r := tool.NewRegistry().
		WithSensitivityEvaluator(approveAlways{}).
		WithHumanApproval(h)
	if err := r.Register(mkSensitiveTool(t, "review", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	res, err := r.Call(tool.Background(), "review", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !h.called {
		t.Fatal("approver not consulted")
	}
	if res == nil || res.IsError {
		t.Fatalf("res = %+v", res)
	}
}

func TestRegistry_Call_RequireApproval_Human_Denies(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.x", Matcher: tool.MatcherExists}}
	r := tool.NewRegistry().
		WithSensitivityEvaluator(approveAlways{}).
		WithHumanApproval(humanNo{})
	if err := r.Register(mkSensitiveTool(t, "review", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "review", []byte(`{"x":1}`))
	if !errors.Is(err, tool.ErrToolDenied) {
		t.Fatalf("want ErrToolDenied, got %v", err)
	}
}

type denyAuthorizer struct{}

func (denyAuthorizer) Authorize(context.Context, tool.ToolCall) (allowed bool, reason string, err error) {
	return false, "no permission", nil
}

func TestRegistry_Call_AuthorizerDenies(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry().WithAuthorizer(denyAuthorizer{})
	if err := r.Register(mkSensitiveTool(t, "x", nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "x", []byte(`{}`))
	if !errors.Is(err, tool.ErrToolDenied) {
		t.Fatalf("want ErrToolDenied, got %v", err)
	}
}

func TestRegistry_PolicyFor_RoundTrip(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry()
	r.WithToolPolicy("a", "policyA")
	if got := r.PolicyFor("a"); got != "policyA" {
		t.Fatalf("got %v", got)
	}
	if got := r.PolicyFor("missing"); got != nil {
		t.Fatalf("got %v", got)
	}
}
