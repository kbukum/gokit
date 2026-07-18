package security

import (
	"context"
	"encoding/json"

	"github.com/kbukum/gokit/observability"
)

// Audit outcome labels recorded for every gated MCP access. They form a small closed vocabulary so downstream sinks can aggregate deterministically.
const (
	OutcomeSuccess            = "success"
	OutcomeDenied             = "denied"
	OutcomeNotFound           = "not_found"
	OutcomeInputTooLarge      = "input_too_large"
	OutcomeResultTooLarge     = "result_too_large"
	OutcomeInvalidInput       = "invalid_input"
	OutcomeOutputInvalid      = "output_validation_error"
	OutcomeAuthorizationError = "authorization_error"
	OutcomeToolError          = "tool_error"
)

// Access kinds distinguish audit records emitted for different protocol surfaces sharing the capability allow-list.
const (
	AccessKindTool     = "tool"
	AccessKindPrompt   = "prompt"
	AccessKindResource = "resource"
)

// ToolAuthorizationRequest is the input to a per-call tool authorization decision. Arguments are the validated, untrusted client payload, carried as raw JSON (never an opaque any). Policy.Authorize forwards the tool identity and, when present, this raw argument payload to the injected authz.Decider under the "mcp_name" and "arguments" context attributes.
type ToolAuthorizationRequest struct {
	// ToolName is the registry tool name (prefix stripped).
	ToolName string
	// MCPName is the exposed MCP tool name (prefix applied).
	MCPName string
	// Arguments are the raw invocation arguments.
	Arguments json.RawMessage
}

// ToolAuditEvent is the final audit record for an MCP tool invocation.
type ToolAuditEvent struct {
	// ToolName is the registry tool name.
	ToolName string
	// MCPName is the exposed MCP tool name.
	MCPName string
	// Outcome is one of the Outcome* labels.
	Outcome string
	// Reason is the decision or policy reason, when present.
	Reason string
	// Error is error text, when present.
	Error string
}

// AuditToolCall records the final outcome of a tool invocation. It is a no-op when no Auditor is configured.
func (p *Policy) AuditToolCall(ctx context.Context, event ToolAuditEvent) {
	if p.Auditor == nil {
		return
	}
	p.Auditor.Audit(ctx, observability.AuditEvent{
		Name: "mcp.tool_call",
		Attributes: map[string]string{
			"kind":    AccessKindTool,
			"tool":    event.ToolName,
			"mcp":     event.MCPName,
			"outcome": event.Outcome,
			"reason":  event.Reason,
			"error":   event.Error,
		},
	})
}

// AuditAccess records a gated prompt, resource, or server-to-client access outcome. It is a no-op when no Auditor is configured.
func (p *Policy) AuditAccess(ctx context.Context, kind, target, outcome, reason string) {
	if p.Auditor == nil {
		return
	}
	p.Auditor.Audit(ctx, observability.AuditEvent{
		Name: "mcp." + kind + "_access",
		Attributes: map[string]string{
			"kind":    kind,
			"target":  target,
			"outcome": outcome,
			"reason":  reason,
		},
	})
}
