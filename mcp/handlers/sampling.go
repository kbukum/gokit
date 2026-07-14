package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// Sample performs a server-to-client sampling request (sampling/createMessage)
// over the given session. The returned message is untrusted model output: when
// a result size limit is configured, an oversized response fails closed rather
// than being handed back to the caller. Every call is audited and bounded by
// ctx (the caller must supply a deadline).
func (h *Handler) Sample(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.CreateMessageParams) (*sdkmcp.CreateMessageResult, error) {
	if ss == nil {
		return nil, fmt.Errorf("mcp: sampling requires an active session")
	}
	res, err := ss.CreateMessage(ctx, params)
	if err != nil {
		h.policy.AuditAccess(ctx, "sampling", "sampling/createMessage", security.OutcomeToolError, err.Error())
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("mcp: sampling returned no result")
	}
	if size := marshaledSize(res.Content); h.policy.ResultTooLarge(size) {
		h.policy.AuditAccess(ctx, "sampling", "sampling/createMessage", security.OutcomeResultTooLarge,
			fmt.Sprintf("sampled content size %d exceeds limit %d", size, h.policy.MaxResultBytes))
		return nil, fmt.Errorf("mcp: sampled content too large: exceeds %d bytes", h.policy.MaxResultBytes)
	}
	h.policy.AuditAccess(ctx, "sampling", "sampling/createMessage", security.OutcomeSuccess, "")
	return res, nil
}

// marshaledSize reports the serialized JSON size of v, or zero when it cannot
// be marshaled (an unmarshalable value carries no measurable payload).
func marshaledSize(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}
