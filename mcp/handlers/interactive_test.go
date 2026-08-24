package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/schema"
)

// finalResult builds a completed (non-round-trip) interactive tool result.
func finalResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

// invokeInteractive drives the hardened interactive handler once with the given
// arguments and (optional) input responses, asserting no protocol-level error.
func invokeInteractive(t *testing.T, h *Handler, toolName string, inputSchema schema.JSON, fn InteractiveToolHandler, params *sdkmcp.CallToolParamsRaw) *sdkmcp.CallToolResult {
	t.Helper()
	handler := h.interactiveToolHandler(toolName, toolName, inputSchema, fn)
	res, err := handler(context.Background(), &sdkmcp.CallToolRequest{Params: params})
	if err != nil {
		t.Fatalf("interactive handler returned protocol error (must be nil): %v", err)
	}
	if res == nil {
		t.Fatal("interactive handler returned nil result")
	}
	return res
}

func okInteractive(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	return finalResult("done"), nil
}

func TestInteractiveHandlerSuccess(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	res := invokeInteractive(t, h, "chat", nil, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat", Arguments: []byte(`{"x":1}`)})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerAllowListDenies(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{
		AllowedTools: security.ToSet([]string{"other"}),
		Auditor:      sink,
	})
	res := invokeInteractive(t, h, "chat", nil, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError || !strings.Contains(resultText(res), "not allowed") {
		t.Fatalf("expected allow-list denial, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerInputTooLarge(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{MaxInputBytes: 4, Auditor: sink})
	res := invokeInteractive(t, h, "chat", nil, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat", Arguments: []byte(`{"prompt":"way too long"}`)})
	if !res.IsError || !strings.Contains(resultText(res), "input too large") {
		t.Fatalf("expected input-too-large, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeInputTooLarge {
		t.Errorf("expected input_too_large outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerSchemaValidation(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	inputSchema := schema.JSON{"type": "object", "required": []any{"prompt"}}
	res := invokeInteractive(t, h, "chat", inputSchema, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat", Arguments: []byte(`{"other":1}`)})
	if !res.IsError || !strings.Contains(resultText(res), "validation error") {
		t.Fatalf("expected schema validation error, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeInvalidInput {
		t.Errorf("expected invalid_input outcome, got %q", sink.last().Attributes["outcome"])
	}
}

// TestInteractiveHandlerReinvocationSkipsValidation proves a retry carrying
// InputResponses bypasses schema validation on the echoed original arguments.
func TestInteractiveHandlerReinvocationSkipsValidation(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	inputSchema := schema.JSON{"type": "object", "required": []any{"prompt"}}
	params := &sdkmcp.CallToolParamsRaw{
		Name:      "chat",
		Arguments: []byte(`{"other":1}`), // would fail validation on a first call
		InputResponses: sdkmcp.InputResponseMap{
			"roots": &sdkmcp.ListRootsResult{},
		},
	}
	res := invokeInteractive(t, h, "chat", inputSchema, okInteractive, params)
	if res.IsError {
		t.Fatalf("re-invocation must skip validation, got error %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerAuthzDeny(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	decider := authz.DeciderFunc(func(_ context.Context, _ authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: false, Reason: "denied by policy"}, nil
	})
	h := newHandler(t, nil, &security.Policy{Decider: decider, Auditor: sink})
	res := invokeInteractive(t, h, "chat", nil, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError || !strings.Contains(resultText(res), "denied by policy") {
		t.Fatalf("expected authz denial, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerAuthzBackendErrorDoesNotLeak(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	decider := authz.DeciderFunc(func(_ context.Context, _ authz.Request) (authz.Decision, error) {
		return authz.Decision{}, errors.New("policy backend unreachable at 10.0.0.9")
	})
	h := newHandler(t, nil, &security.Policy{Decider: decider, Auditor: sink})
	res := invokeInteractive(t, h, "chat", nil, okInteractive,
		&sdkmcp.CallToolParamsRaw{Name: "chat"})
	if got := resultText(res); got != "authorization error" {
		t.Fatalf("expected generic authorization error, got %q", got)
	}
	last := sink.last()
	if last.Attributes["outcome"] != security.OutcomeAuthorizationError {
		t.Errorf("expected authorization_error outcome, got %q", last.Attributes["outcome"])
	}
	if !strings.Contains(last.Attributes["error"], "10.0.0.9") {
		t.Errorf("audit must record the real backend error, got %q", last.Attributes["error"])
	}
}

// TestInteractiveHandlerInputRequiredRoundTrip proves a result carrying a
// non-nil InputRequests map is passed through as an input-required round trip
// and audited as such, without a result-size check.
func TestInteractiveHandlerInputRequiredRoundTrip(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{MaxResultBytes: 1, Auditor: sink})
	fn := func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{InputRequests: sdkmcp.InputRequestMap{}}, nil
	}
	res := invokeInteractive(t, h, "chat", nil, fn, &sdkmcp.CallToolParamsRaw{Name: "chat"})
	if res.IsError {
		t.Fatalf("input-required round trip must not be an error result: %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeInputRequired {
		t.Errorf("expected input_required outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerResultTooLarge(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{MaxResultBytes: 3, Auditor: sink})
	fn := func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return finalResult("a very large final payload"), nil
	}
	res := invokeInteractive(t, h, "chat", nil, fn, &sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError || !strings.Contains(resultText(res), "result too large") {
		t.Fatalf("expected result-too-large, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerFnError(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	fn := func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return nil, errors.New("handler blew up")
	}
	res := invokeInteractive(t, h, "chat", nil, fn, &sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError || !strings.Contains(resultText(res), "handler blew up") {
		t.Fatalf("expected fn error surfaced, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerNilResultFailsClosed(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	fn := func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return nil, nil //nolint:nilnil // models an interactive tool returning no result
	}
	res := invokeInteractive(t, h, "chat", nil, fn, &sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError || !strings.Contains(resultText(res), "no result") {
		t.Fatalf("nil interactive result must fail closed, got %q", resultText(res))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error outcome, got %q", sink.last().Attributes["outcome"])
	}
}

func TestInteractiveHandlerErrorResultPassthrough(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := newHandler(t, nil, &security.Policy{Auditor: sink})
	fn := func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{
			IsError: true,
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "tool-side failure"}},
		}, nil
	}
	res := invokeInteractive(t, h, "chat", nil, fn, &sdkmcp.CallToolParamsRaw{Name: "chat"})
	if !res.IsError {
		t.Fatal("error result must pass through as error")
	}
	last := sink.last()
	if last.Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error outcome, got %q", last.Attributes["outcome"])
	}
	if !strings.Contains(last.Attributes["error"], "tool-side failure") {
		t.Errorf("audit must record the interactive error text, got %q", last.Attributes["error"])
	}
}
