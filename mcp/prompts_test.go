package mcp_test

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	kitMcp "github.com/kbukum/gokit/mcp"
	"github.com/kbukum/gokit/tool"
)

func staticPrompt(name, text string) (*sdkmcp.Prompt, sdkmcp.PromptHandler) {
	return &sdkmcp.Prompt{Name: name, Description: "d"},
		func(_ context.Context, _ *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			return &sdkmcp.GetPromptResult{
				Messages: []*sdkmcp.PromptMessage{{Role: "user", Content: &sdkmcp.TextContent{Text: text}}},
			}, nil
		}
}

// TestPromptGetRoundTrip proves a registered prompt is reachable end-to-end
// over the transport. Allow-list and audit gating are covered by the handlers
// package unit tests.
func TestPromptGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	prompt, handler := staticPrompt("summary", "Summarize this")
	server, err := kitMcp.NewServer("s", "1.0.0", tool.NewRegistry(), kitMcp.WithPrompt(prompt, handler))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	p := connectPeer(t, ctx, server, nil)

	res, err := p.clientSession.GetPrompt(ctx, &sdkmcp.GetPromptParams{Name: "summary"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(res.Messages))
	}
}
