package mcp_test

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/tool"
)

func elicitSchema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
	}
}

// TestInteractiveElicitRoundTrip drives a full multi round-trip: the interactive
// tool asks the client for a name via elicitation, then produces a greeting from
// the response. The SDK client middleware fulfills the request transparently.
func TestInteractiveElicitRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "greet"}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if len(req.Params.InputResponses) == 0 {
			return &sdkmcp.CallToolResult{
				InputRequests: sdkmcp.InputRequestMap{
					"user_name": &sdkmcp.ElicitParams{Message: "name?", RequestedSchema: elicitSchema()},
				},
				RequestState: "step=1",
			}, nil
		}
		res, err := server.ElicitResponse(ctx, req, "user_name")
		if err != nil {
			return nil, err
		}
		name, _ := res.Content["name"].(string)
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "Hello " + name}}}, nil
	})

	p := connectPeer(t, ctx, server, elicitClientOpts(map[string]any{"name": "MCP Go"}))

	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "greet"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out.IsError {
		t.Fatalf("unexpected error result: %+v", out.Content)
	}
	tc, ok := out.Content[0].(*sdkmcp.TextContent)
	if !ok || tc.Text != "Hello MCP Go" {
		t.Fatalf("unexpected content: %+v", out.Content)
	}
	if audit.outcomeFor("elicitation/create") != security.OutcomeSuccess {
		t.Errorf("expected elicitation success audit, got %q", audit.outcomeFor("elicitation/create"))
	}
}

// TestInteractiveSampleRoundTrip drives a sampling round trip through the
// interactive tool path and asserts the model output is returned and audited.
func TestInteractiveSampleRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "ask"}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if len(req.Params.InputResponses) == 0 {
			return &sdkmcp.CallToolResult{
				InputRequests: sdkmcp.InputRequestMap{"draft": sampleParams()},
				RequestState:  "step=1",
			}, nil
		}
		res, err := server.SampleResponse(ctx, req, "draft")
		if err != nil {
			return nil, err
		}
		tc, _ := res.Content[0].(*sdkmcp.TextContent)
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: tc.Text}}}, nil
	})

	p := connectPeer(t, ctx, server, samplingClientOpts("sampled reply"))

	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "ask"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out.IsError {
		t.Fatalf("unexpected error result: %+v", out.Content)
	}
	tc, ok := out.Content[0].(*sdkmcp.TextContent)
	if !ok || tc.Text != "sampled reply" {
		t.Fatalf("unexpected content: %+v", out.Content)
	}
	if audit.outcomeFor("sampling/createMessage") != security.OutcomeSuccess {
		t.Errorf("expected sampling success audit, got %q", audit.outcomeFor("sampling/createMessage"))
	}
}

// TestInteractiveRootsRoundTrip drives a roots/list round trip through the
// interactive tool path.
func TestInteractiveRootsRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "roots"}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if len(req.Params.InputResponses) == 0 {
			return &sdkmcp.CallToolResult{
				InputRequests: sdkmcp.InputRequestMap{"roots": &sdkmcp.ListRootsParams{}},
				RequestState:  "step=1",
			}, nil
		}
		res, err := server.RootsResponse(ctx, req, "roots")
		if err != nil {
			return nil, err
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: res.Roots[0].URI}}}, nil
	})

	p := connectPeer(t, ctx, server, nil, &sdkmcp.Root{URI: "file:///workspace", Name: "workspace"})

	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "roots"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if out.IsError {
		t.Fatalf("unexpected error result: %+v", out.Content)
	}
	tc, ok := out.Content[0].(*sdkmcp.TextContent)
	if !ok || tc.Text != "file:///workspace" {
		t.Fatalf("unexpected content: %+v", out.Content)
	}
	if audit.outcomeFor("roots/list") != security.OutcomeSuccess {
		t.Errorf("expected roots success audit, got %q", audit.outcomeFor("roots/list"))
	}
}

// TestInteractiveElicitResponseTooLarge asserts the elicited content is size
// limited: an oversized response fails closed and is audited as result_too_large.
func TestInteractiveElicitResponseTooLarge(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(),
		kitMcp.WithMaxResultBytes(4), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "greet"}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if len(req.Params.InputResponses) == 0 {
			return &sdkmcp.CallToolResult{
				InputRequests: sdkmcp.InputRequestMap{"user_name": &sdkmcp.ElicitParams{Message: "name?", RequestedSchema: elicitSchema()}},
				RequestState:  "step=1",
			}, nil
		}
		if _, err := server.ElicitResponse(ctx, req, "user_name"); err != nil {
			return nil, err
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
	})

	p := connectPeer(t, ctx, server, elicitClientOpts(map[string]any{"name": strings.Repeat("x", 128)}))

	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "greet"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !out.IsError {
		t.Fatal("oversized elicited content must fail closed")
	}
	if audit.outcomeFor("elicitation/create") != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large audit, got %q", audit.outcomeFor("elicitation/create"))
	}
}

// TestInteractiveMissingResponse asserts a missing input response fails closed.
func TestInteractiveMissingResponse(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "greet"}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		// Wrong label: the response was keyed "user_name" but we read "missing".
		if _, err := server.ElicitResponse(ctx, req, "missing"); err != nil {
			return nil, err
		}
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
	})

	p := connectPeer(t, ctx, server, elicitClientOpts(map[string]any{"name": "x"}))
	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "greet"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !out.IsError {
		t.Fatal("missing input response must fail closed")
	}
}

// TestInteractiveInputInvalid asserts arguments are validated against the tool's
// declared input schema before the handler runs.
func TestInteractiveInputInvalid(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	called := false
	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"n": map[string]any{"type": "integer"}},
		"required":             []any{"n"},
		"additionalProperties": false,
	}
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "calc", InputSchema: schema}, func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		called = true
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
	})

	p := connectPeer(t, ctx, server, nil)
	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "calc", Arguments: map[string]any{"n": "not-an-int"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !out.IsError {
		t.Fatal("invalid arguments must fail closed")
	}
	if called {
		t.Fatal("handler must not run on invalid input")
	}
	if audit.outcomeFor("calc") != security.OutcomeInvalidInput {
		t.Errorf("expected invalid_input audit, got %q", audit.outcomeFor("calc"))
	}
}

func TestInteractiveToolDenied(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(),
		kitMcp.WithAllowedTools("other"), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	called := false
	server.AddInteractiveTool(&sdkmcp.Tool{Name: "greet"}, func(_ context.Context, _ *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		called = true
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
	})

	p := connectPeer(t, ctx, server, nil)
	out, err := p.clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: "greet"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !out.IsError {
		t.Fatal("tool outside the allow-list must be denied")
	}
	if called {
		t.Fatal("denied interactive tool handler must not run")
	}
	if audit.outcomeFor("greet") != security.OutcomeDenied {
		t.Errorf("expected denied audit, got %q", audit.outcomeFor("greet"))
	}
}
