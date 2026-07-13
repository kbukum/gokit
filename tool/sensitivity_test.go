package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/resilience"
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

type errAuthorizer struct{}

func (errAuthorizer) Authorize(context.Context, tool.ToolCall) (allowed bool, reason string, err error) {
	return false, "", errors.New("authz backend down")
}

func TestRegistry_Call_AuthorizerError(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry().WithAuthorizer(errAuthorizer{})
	if err := r.Register(mkSensitiveTool(t, "x", nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "x", []byte(`{}`))
	if err == nil || errors.Is(err, tool.ErrToolDenied) {
		t.Fatalf("want internal authorize error, got %v", err)
	}
	if !strings.Contains(err.Error(), "authz backend down") {
		t.Fatalf("error should wrap cause: %v", err)
	}
}

type errEvaluator struct{}

func (errEvaluator) Evaluate(context.Context, tool.ToolCall, tool.SensitivePredicate) (tool.Decision, string, error) {
	return "", "", errors.New("evaluator exploded")
}

func TestRegistry_Call_EvaluatorError(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.x", Matcher: tool.MatcherExists}}
	r := tool.NewRegistry().WithSensitivityEvaluator(errEvaluator{})
	if err := r.Register(mkSensitiveTool(t, "x", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "x", []byte(`{"x":1}`))
	if err == nil || !strings.Contains(err.Error(), "evaluator exploded") {
		t.Fatalf("want wrapped evaluate error, got %v", err)
	}
}

type unknownEvaluator struct{}

func (unknownEvaluator) Evaluate(context.Context, tool.ToolCall, tool.SensitivePredicate) (tool.Decision, string, error) {
	return tool.Decision("bogus"), "", nil
}

func TestRegistry_Call_UnknownDecisionFailsClosed(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.x", Matcher: tool.MatcherExists}}
	r := tool.NewRegistry().WithSensitivityEvaluator(unknownEvaluator{})
	if err := r.Register(mkSensitiveTool(t, "x", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "x", []byte(`{"x":1}`))
	if err == nil || !strings.Contains(err.Error(), "unknown evaluator decision") {
		t.Fatalf("want unknown-decision error, got %v", err)
	}
}

type errApprover struct{}

func (errApprover) Approve(context.Context, tool.ToolCall) (bool, error) {
	return false, errors.New("approver offline")
}

func TestRegistry_Call_ApproverError(t *testing.T) {
	t.Parallel()
	preds := []tool.SensitivePredicate{{JSONPath: "$.x", Matcher: tool.MatcherExists}}
	r := tool.NewRegistry().
		WithSensitivityEvaluator(approveAlways{}).
		WithHumanApproval(errApprover{})
	if err := r.Register(mkSensitiveTool(t, "x", preds)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := r.Call(tool.Background(), "x", []byte(`{"x":1}`))
	if err == nil || !strings.Contains(err.Error(), "approver offline") {
		t.Fatalf("want wrapped approval error, got %v", err)
	}
}

func TestRegistry_WithToolPolicy_EmptyNameIgnored(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry()
	r.WithToolPolicy("", resilience.NewPolicy())
	if got := r.PolicyFor(""); got != nil {
		t.Fatalf("empty name should not store a policy, got %v", got)
	}
}

func TestRegistry_PolicyFor_RoundTrip(t *testing.T) {
	t.Parallel()
	r := tool.NewRegistry()
	policy := resilience.NewPolicy().WithTimeout(time.Second)
	r.WithToolPolicy("a", policy)
	if got := r.PolicyFor("a"); got != policy {
		t.Fatalf("got %v, want %v", got, policy)
	}
	if got := r.PolicyFor("missing"); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
