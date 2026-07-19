package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// Elicit performs a server-to-client elicitation request over the given session.
// The submitted content is untrusted user input: when a result size limit is configured,
// oversized content fails closed. Every call is audited and bounded by ctx.
func (h *Handler) Elicit(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ElicitParams) (*sdkmcp.ElicitResult, error) {
	if ss == nil {
		return nil, fmt.Errorf("mcp: elicitation requires an active session")
	}
	res, err := ss.Elicit(ctx, params)
	if err != nil {
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindElicitation, Target: "elicitation/create", Outcome: security.OutcomeToolError, Reason: err.Error()})
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("mcp: elicitation returned no result")
	}
	if reason, tooLarge := h.contentTooLarge(res.Content); tooLarge {
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindElicitation, Target: "elicitation/create", Outcome: security.OutcomeResultTooLarge, Reason: "elicited content " + reason})
		return nil, fmt.Errorf("mcp: elicited content too large: %s", reason)
	}
	h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindElicitation, Target: "elicitation/create", Outcome: security.OutcomeSuccess, Reason: res.Action})
	return res, nil
}
