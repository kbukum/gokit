package mcp

import (
	"context"
	"encoding/json"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/observability"
)

type ToolAuthorizationRequest struct {
	ToolName  string
	MCPName   string
	Arguments json.RawMessage
	Session   any
}
type ToolAuditEvent struct {
	ToolName string
	MCPName  string
	Outcome  string
	Reason   string
	Error    string
}

func authzRequest(req ToolAuthorizationRequest) authz.Request {
	return authz.Request{Resource: authz.Resource{Type: "tool", ID: req.ToolName}, Action: authz.ActionToolInvoke, Context: authz.Attributes{"mcp_name": req.MCPName}}
}

func audit(ctx context.Context, auditor observability.Auditor, event ToolAuditEvent) {
	if auditor == nil {
		return
	}
	auditor.Audit(ctx, observability.AuditEvent{Name: "mcp.tool_call", Attributes: map[string]string{"tool": event.ToolName, "mcp": event.MCPName, "outcome": event.Outcome, "reason": event.Reason, "error": event.Error}})
}
