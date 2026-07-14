package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

type addIn struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addOut struct {
	Sum int `json:"sum"`
}

func addTool() tool.Callable {
	return tool.FromFunc("add", "Add", func(_ context.Context, in addIn) (addOut, error) {
		return addOut{Sum: in.A + in.B}, nil
	}).AsCallable()
}

// nilResultCallable models an untrusted tool that returns (nil, nil); the
// handler must fail closed rather than dereference the nil result.
type nilResultCallable struct{}

func (nilResultCallable) Definition() tool.Definition { return tool.Definition{Name: "void"} }

func (nilResultCallable) Validate(json.RawMessage) schema.ValidationResult {
	return schema.ValidationResult{Valid: true}
}

func (nilResultCallable) Call(*tool.Context, json.RawMessage) (*tool.Result, error) {
	return nil, nil //nolint:nilnil // models an untrusted tool returning no result
}

// auditSink is a concurrency-safe capturing auditor for handler tests.
type auditSink struct {
	mu     sync.Mutex
	events []observability.AuditEvent
}

func (a *auditSink) Audit(_ context.Context, e observability.AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditSink) last() observability.AuditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.events) == 0 {
		return observability.AuditEvent{}
	}
	return a.events[len(a.events)-1]
}

func regWith(t *testing.T, callables ...tool.Callable) *tool.Registry {
	t.Helper()
	reg := tool.NewRegistry()
	for _, c := range callables {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.Definition().Name, err)
		}
	}
	return reg
}

func newHandler(t *testing.T, reg *tool.Registry, policy *security.Policy) *Handler {
	t.Helper()
	sdk := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	return New(sdk, reg, policy, "")
}

// invoke runs the hardened tool handler directly for deterministic branch coverage.
func invoke(t *testing.T, h *Handler, toolName, mcpName string, args json.RawMessage) *sdkmcp.CallToolResult {
	t.Helper()
	handler := h.makeToolHandler(toolName, mcpName)
	req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: mcpName, Arguments: args}}
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned protocol error (must be nil): %v", err)
	}
	if res == nil {
		t.Fatal("handler returned nil result")
	}
	return res
}

func resultText(r *sdkmcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolHandlerSuccess(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":2,"b":5}`))
	if res.IsError {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerNilResultFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, nilResultCallable{}), &security.Policy{Auditor: sink})
	res := invoke(t, h, "void", "void", nil)
	if !res.IsError {
		t.Fatal("nil tool result must fail closed with an error result")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerAllowListDenies(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{
		AllowedTools: security.ToSet([]string{"other"}),
		Auditor:      sink,
	})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":1,"b":1}`))
	if !res.IsError {
		t.Fatal("expected denial error result")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerInputTooLarge(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{MaxInputBytes: 5, Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":100000,"b":200000}`))
	if !res.IsError || !strings.Contains(resultText(res), "input too large") {
		t.Fatalf("expected input-too-large, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeInputTooLarge {
		t.Errorf("expected input_too_large outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerNotFound(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "ghost", "ghost", json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(resultText(res), "tool not found") {
		t.Fatalf("expected not-found, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeNotFound {
		t.Errorf("expected not_found outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerInvalidInput(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":"not-a-number"}`))
	if !res.IsError || !strings.Contains(resultText(res), "validation error") {
		t.Fatalf("expected validation error, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeInvalidInput {
		t.Errorf("expected invalid_input outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerAuthzDeny(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	decider := authz.DeciderFunc(func(_ context.Context, _ authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: false, Reason: "not permitted"}, nil
	})
	h := newHandler(t, regWith(t, addTool()), &security.Policy{Decider: decider, Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":1,"b":1}`))
	if !res.IsError || !strings.Contains(resultText(res), "not permitted") {
		t.Fatalf("expected authz denial, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerAuthzBackendError(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	decider := authz.DeciderFunc(func(_ context.Context, _ authz.Request) (authz.Decision, error) {
		return authz.Decision{}, errors.New("policy backend unreachable at 10.0.0.1")
	})
	h := newHandler(t, regWith(t, addTool()), &security.Policy{Decider: decider, Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":1,"b":1}`))
	// Caller sees a generic message; the real backend error must not leak.
	if got := resultText(res); got != "authorization error" {
		t.Fatalf("expected generic authorization error, got %q", got)
	}
	last := sink.last()
	if last.Attributes["outcome"] != security.OutcomeAuthorizationError {
		t.Errorf("expected authorization_error outcome, got %q", last.Attributes["outcome"])
	}
	if !strings.Contains(last.Attributes["error"], "10.0.0.1") {
		t.Errorf("audit must record the real backend error, got %q", last.Attributes["error"])
	}
}

func TestToolHandlerResultTooLarge(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, regWith(t, addTool()), &security.Policy{MaxResultBytes: 3, Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":1000,"b":2000}`))
	if !res.IsError || !strings.Contains(resultText(res), "result too large") {
		t.Fatalf("expected result-too-large, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerOutputValidation(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	tl := tool.FromFunc("add", "Add", func(_ context.Context, in addIn) (addOut, error) {
		return addOut{Sum: in.A + in.B}, nil
	})
	// Force an output schema the real output cannot satisfy.
	tl.Def.OutputSchema = schema.JSON{"type": "object", "required": []any{"missing_field"}}
	h := newHandler(t, regWith(t, tl.AsCallable()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "add", "add", json.RawMessage(`{"a":1,"b":1}`))
	if !res.IsError || !strings.Contains(resultText(res), "output validation error") {
		t.Fatalf("expected output validation error, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeOutputInvalid {
		t.Errorf("expected output_validation_error outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerDestructiveGate(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	tl := tool.FromFunc("wipe", "Delete everything", func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	})
	tl.Def.Envelope.Safety = tool.SafetyDestructive
	// Default registry uses DenyHumanApproval, so the destructive call fails closed.
	h := newHandler(t, regWith(t, tl.AsCallable()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "wipe", "wipe", json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatal("destructive tool must be gated without human approval")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied outcome for destructive gate, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerToolError(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	tl := tool.FromFunc("boom", "Fails", func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, errors.New("kaboom")
	})
	h := newHandler(t, regWith(t, tl.AsCallable()), &security.Policy{Auditor: sink})
	res := invoke(t, h, "boom", "boom", json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(resultText(res), "kaboom") {
		t.Fatalf("expected tool error surfaced as error result, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestToolHandlerContextCancelled(t *testing.T) {
	t.Parallel()
	blocked := make(chan struct{})
	tl := tool.FromFunc("slow", "Blocks on context", func(ctx context.Context, _ struct{}) (struct{}, error) {
		<-ctx.Done()
		return struct{}{}, ctx.Err()
	})
	h := newHandler(t, regWith(t, tl.AsCallable()), &security.Policy{})
	handler := h.makeToolHandler("slow", "slow")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		close(blocked)
		cancel()
	}()
	<-blocked
	req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: "slow", Arguments: json.RawMessage(`{}`)}}
	res, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler returned protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatal("canceled call must surface as error result")
	}
}
