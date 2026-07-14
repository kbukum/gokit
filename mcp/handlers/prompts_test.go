package handlers

import (
	"context"
	"errors"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/tool"
)

func promptEntry(name, text string) PromptEntry {
	return PromptEntry{
		Prompt: &sdkmcp.Prompt{Name: name, Description: "d"},
		Handler: func(_ context.Context, _ *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			return &sdkmcp.GetPromptResult{
				Messages: []*sdkmcp.PromptMessage{{Role: "user", Content: &sdkmcp.TextContent{Text: text}}},
			}, nil
		},
	}
}

func promptHandler(t *testing.T, policy *security.Policy) *Handler {
	t.Helper()
	sdk := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	return New(sdk, tool.NewRegistry(), policy, "")
}

func TestWrapPromptHandlerAllows(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := promptHandler(t, &security.Policy{Auditor: sink})
	wrapped := h.wrapPromptHandler(promptEntry("summary", "text"))
	res, err := wrapped(context.Background(), &sdkmcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("wrapped prompt: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(res.Messages))
	}
	if sink.last().Attributes["outcome"] != security.OutcomeSuccess {
		t.Errorf("expected success audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestWrapPromptHandlerDenies(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := promptHandler(t, &security.Policy{
		AllowedPrompts: security.ToSet([]string{"allowed-only"}),
		Auditor:        sink,
	})
	wrapped := h.wrapPromptHandler(promptEntry("blocked", "secret"))
	if _, err := wrapped(context.Background(), &sdkmcp.GetPromptRequest{}); err == nil {
		t.Fatal("expected denial for prompt not in allow-list")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeDenied {
		t.Errorf("expected denied audit, got %q", sink.last().Attributes["outcome"])
	}
}

func TestWrapPromptHandlerErrorAudited(t *testing.T) {
	t.Parallel()
	sink := &auditSink{}
	h := promptHandler(t, &security.Policy{Auditor: sink})
	entry := PromptEntry{
		Prompt: &sdkmcp.Prompt{Name: "boom", Description: "d"},
		Handler: func(_ context.Context, _ *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
			return nil, errors.New("handler failed")
		},
	}
	wrapped := h.wrapPromptHandler(entry)
	if _, err := wrapped(context.Background(), &sdkmcp.GetPromptRequest{}); err == nil {
		t.Fatal("expected handler error to propagate")
	}
	if sink.last().Attributes["outcome"] != security.OutcomeToolError {
		t.Errorf("expected tool_error audit, got %q", sink.last().Attributes["outcome"])
	}
}
