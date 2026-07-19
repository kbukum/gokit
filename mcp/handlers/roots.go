package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// ListRoots asks the connected client for its roots over the given session.
// The call is bounded by ctx and audited.
// It fails closed when the session is nil rather than dereferencing it.
func (h *Handler) ListRoots(ctx context.Context, ss *sdkmcp.ServerSession, params *sdkmcp.ListRootsParams) (*sdkmcp.ListRootsResult, error) {
	if ss == nil {
		return nil, fmt.Errorf("mcp: roots/list requires an active session")
	}
	res, err := ss.ListRoots(ctx, params)
	if err != nil {
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindRoots, Target: "roots/list", Outcome: security.OutcomeToolError, Reason: err.Error()})
		return nil, err
	}
	h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindRoots, Target: "roots/list", Outcome: security.OutcomeSuccess, Reason: ""})
	return res, nil
}
