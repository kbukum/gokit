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

// --- Sampling ---

func samplingClientOpts(text string) *sdkmcp.ClientOptions {
	return &sdkmcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error) {
			return &sdkmcp.CreateMessageResult{
				Model:   "test-model",
				Role:    "assistant",
				Content: &sdkmcp.TextContent{Text: text},
			}, nil
		},
	}
}

func sampleParams() *sdkmcp.CreateMessageParams {
	return &sdkmcp.CreateMessageParams{
		MaxTokens: 100,
		Messages: []*sdkmcp.SamplingMessage{
			{Role: "user", Content: &sdkmcp.TextContent{Text: "hi"}},
		},
	}
}

func TestSampleRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, samplingClientOpts("sampled reply"))

	res, err := p.server.Sample(ctx, p.serverSession, sampleParams())
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	tc, ok := res.Content.(*sdkmcp.TextContent)
	if !ok || tc.Text != "sampled reply" {
		t.Fatalf("unexpected sampled content: %+v", res.Content)
	}
	if audit.outcomeFor("sampling/createMessage") != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", audit.outcomeFor("sampling/createMessage"))
	}
}

func TestSampleResultTooLarge(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(),
		kitMcp.WithMaxResultBytes(4), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, samplingClientOpts(strings.Repeat("x", 128)))

	if _, err := p.server.Sample(ctx, p.serverSession, sampleParams()); err == nil {
		t.Fatal("oversized sampled content must fail closed")
	}
	if audit.outcomeFor("sampling/createMessage") != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large audit, got %q", audit.outcomeFor("sampling/createMessage"))
	}
}

func TestSampleNilSession(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := server.Sample(context.Background(), nil, sampleParams()); err == nil {
		t.Fatal("nil session must fail closed")
	}
}

func TestSampleClientError(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	clientOpts := &sdkmcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *sdkmcp.CreateMessageRequest) (*sdkmcp.CreateMessageResult, error) {
			return nil, context.Canceled
		},
	}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, clientOpts)
	if _, err := p.server.Sample(ctx, p.serverSession, sampleParams()); err == nil {
		t.Fatal("expected client-side sampling error to propagate")
	}
	if audit.outcomeFor("sampling/createMessage") != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", audit.outcomeFor("sampling/createMessage"))
	}
}

// --- Elicitation ---

func elicitClientOpts(content map[string]any) *sdkmcp.ClientOptions {
	return &sdkmcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *sdkmcp.ElicitRequest) (*sdkmcp.ElicitResult, error) {
			return &sdkmcp.ElicitResult{Action: "accept", Content: content}, nil
		},
	}
}

func elicitParams() *sdkmcp.ElicitParams {
	return &sdkmcp.ElicitParams{
		Mode:    "form",
		Message: "Please confirm",
		RequestedSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		},
	}
}

func TestElicitRoundTrip(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, elicitClientOpts(map[string]any{"ok": true}))

	res, err := p.server.Elicit(ctx, p.serverSession, elicitParams())
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if res.Action != "accept" {
		t.Fatalf("unexpected action: %q", res.Action)
	}
	if audit.outcomeFor("elicitation/create") != security.OutcomeSuccess {
		t.Errorf("expected success audit outcome, got %q", audit.outcomeFor("elicitation/create"))
	}
	if audit.reasonFor("elicitation/create") != "accept" {
		t.Errorf("expected accept audit reason, got %q", audit.reasonFor("elicitation/create"))
	}
}

func TestElicitResultTooLarge(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(),
		kitMcp.WithMaxResultBytes(4), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, elicitClientOpts(map[string]any{"blob": strings.Repeat("x", 128)}))

	if _, err := p.server.Elicit(ctx, p.serverSession, elicitParams()); err == nil {
		t.Fatal("oversized elicited content must fail closed")
	}
	if audit.outcomeFor("elicitation/create") != security.OutcomeResultTooLarge {
		t.Errorf("expected result_too_large audit, got %q", audit.outcomeFor("elicitation/create"))
	}
}

func TestElicitNilSession(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := server.Elicit(context.Background(), nil, elicitParams()); err == nil {
		t.Fatal("nil session must fail closed")
	}
}

func TestElicitClientError(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	clientOpts := &sdkmcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *sdkmcp.ElicitRequest) (*sdkmcp.ElicitResult, error) {
			return nil, context.Canceled
		},
	}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, clientOpts)
	if _, err := p.server.Elicit(ctx, p.serverSession, elicitParams()); err == nil {
		t.Fatal("expected client-side elicitation error to propagate")
	}
	if audit.outcomeFor("elicitation/create") != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", audit.outcomeFor("elicitation/create"))
	}
}

// --- Roots ---

func TestListRoots(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil,
		&sdkmcp.Root{URI: "file:///workspace", Name: "workspace"})

	res, err := p.server.ListRoots(ctx, p.serverSession, nil)
	if err != nil {
		t.Fatalf("ListRoots: %v", err)
	}
	if len(res.Roots) != 1 || res.Roots[0].URI != "file:///workspace" {
		t.Fatalf("unexpected roots: %+v", res.Roots)
	}
	if audit.outcomeFor("roots/list") != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", audit.outcomeFor("roots/list"))
	}
}

func TestListRootsNilSession(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if _, err := server.ListRoots(context.Background(), nil, nil); err == nil {
		t.Fatal("nil session must fail closed")
	}
}

func TestListRootsClientError(t *testing.T) {
	ctx := context.Background()
	audit := &captureAuditor{}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithAuditor(audit))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil, &sdkmcp.Root{URI: "file:///w", Name: "w"})

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.server.ListRoots(canceledCtx, p.serverSession, nil); err == nil {
		t.Fatal("expected roots/list to fail on a canceled context")
	}
	if audit.outcomeFor("roots/list") != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", audit.outcomeFor("roots/list"))
	}
}

// --- Logging and progress ---

func TestLogAndProgress(t *testing.T) {
	ctx := context.Background()
	logs := make(chan string, 1)
	progress := make(chan float64, 1)
	clientOpts := &sdkmcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, r *sdkmcp.LoggingMessageRequest) {
			if s, ok := r.Params.Data.(string); ok {
				logs <- s
			} else {
				logs <- ""
			}
		},
		ProgressNotificationHandler: func(_ context.Context, r *sdkmcp.ProgressNotificationClientRequest) {
			progress <- r.Params.Progress
		},
	}
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, clientOpts)

	if err := p.clientSession.SetLoggingLevel(ctx, &sdkmcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	if err := p.server.Log(ctx, p.serverSession, &sdkmcp.LoggingMessageParams{Level: "info", Data: "hello"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if got := <-logs; got != "hello" {
		t.Errorf("log data: got %q", got)
	}

	if err := p.server.NotifyProgress(ctx, p.serverSession, &sdkmcp.ProgressNotificationParams{ProgressToken: "t1", Progress: 0.5}); err != nil {
		t.Fatalf("NotifyProgress: %v", err)
	}
	if got := <-progress; got != 0.5 {
		t.Errorf("progress: got %v", got)
	}
}

func TestLogNilSession(t *testing.T) {
	t.Parallel()
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := server.Log(context.Background(), nil, &sdkmcp.LoggingMessageParams{Level: "info"}); err == nil {
		t.Fatal("nil session must fail closed for Log")
	}
	if err := server.NotifyProgress(context.Background(), nil, &sdkmcp.ProgressNotificationParams{}); err == nil {
		t.Fatal("nil session must fail closed for NotifyProgress")
	}
}
