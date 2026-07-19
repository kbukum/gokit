package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// Sample performs a server-to-client sampling request (sampling/createMessage) over the given session.
// The returned message is untrusted model output: when a result size limit is configured,
// an oversized response fails closed rather than being handed back to the caller.
// Every call is audited and bounded by ctx (the caller must supply a deadline).
func (h *Handler) Sample(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.CreateMessageParams) (*sdkmcp.CreateMessageResult, error) {
	if ss == nil {
		return nil, fmt.Errorf("mcp: sampling requires an active session")
	}
	res, err := ss.CreateMessage(ctx, params)
	if err != nil {
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindSampling, Target: "sampling/createMessage", Outcome: security.OutcomeToolError, Reason: err.Error()})
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("mcp: sampling returned no result")
	}
	if reason, tooLarge := h.contentTooLarge(res.Content); tooLarge {
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindSampling, Target: "sampling/createMessage", Outcome: security.OutcomeResultTooLarge, Reason: "sampled content " + reason})
		return nil, fmt.Errorf("mcp: sampled content too large: %s", reason)
	}
	h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindSampling, Target: "sampling/createMessage", Outcome: security.OutcomeSuccess, Reason: ""})
	return res, nil
}
