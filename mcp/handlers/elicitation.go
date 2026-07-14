package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// Elicit performs a server-to-client elicitation request over the given
// session. The submitted content is untrusted user input: when a result size
// limit is configured, oversized content fails closed. Every call is audited
// and bounded by ctx.
func (h *Handler) Elicit(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ElicitParams) (*sdkmcp.ElicitResult, error) {
	if ss == nil {
		return nil, fmt.Errorf("mcp: elicitation requires an active session")
	}
	res, err := ss.Elicit(ctx, params)
	if err != nil {
		h.policy.AuditAccess(ctx, "elicitation", "elicitation/create", security.OutcomeToolError, err.Error())
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("mcp: elicitation returned no result")
	}
	if size := marshaledSize(res.Content); h.policy.ResultTooLarge(size) {
		h.policy.AuditAccess(ctx, "elicitation", "elicitation/create", security.OutcomeResultTooLarge,
			fmt.Sprintf("elicited content size %d exceeds limit %d", size, h.policy.MaxResultBytes))
		return nil, fmt.Errorf("mcp: elicited content too large: exceeds %d bytes", h.policy.MaxResultBytes)
	}
	h.policy.AuditAccess(ctx, "elicitation", "elicitation/create", security.OutcomeSuccess, res.Action)
	return res, nil
}
