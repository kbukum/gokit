package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// PromptEntry is a static MCP prompt registration.
type PromptEntry struct {
	Prompt  *sdkmcp.Prompt
	Handler sdkmcp.PromptHandler
}

// wrapPromptHandler enforces the prompt allow-list and audits the outcome
// around the caller-supplied handler.
func (h *Handler) wrapPromptHandler(entry PromptEntry) sdkmcp.PromptHandler {
	name := entry.Prompt.Name
	return func(ctx context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		if !h.policy.AllowsPrompt(name) {
			h.policy.AuditAccess(ctx, security.AccessKindPrompt, name, security.OutcomeDenied, "not in allow-list")
			return nil, fmt.Errorf("prompt not allowed: %s", name)
		}
		res, err := entry.Handler(ctx, req)
		if err != nil {
			h.policy.AuditAccess(ctx, security.AccessKindPrompt, name, security.OutcomeToolError, err.Error())
			return nil, err
		}
		h.policy.AuditAccess(ctx, security.AccessKindPrompt, name, security.OutcomeSuccess, "")
		return res, nil
	}
}
