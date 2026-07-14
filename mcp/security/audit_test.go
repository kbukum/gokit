package security

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/observability"
)

func TestAuditIsNilSafe(t *testing.T) {
	t.Parallel()
	p := &Policy{}
	// Must not panic without an auditor configured.
	p.AuditToolCall(context.Background(), ToolAuditEvent{ToolName: "x"})
	p.AuditAccess(context.Background(), AccessKindPrompt, "p", OutcomeDenied, "r")

	var events []observability.AuditEvent
	p.Auditor = observability.AuditorFunc(func(_ context.Context, e observability.AuditEvent) {
		events = append(events, e)
	})
	p.AuditToolCall(context.Background(), ToolAuditEvent{ToolName: "add", MCPName: "svc_add", Outcome: OutcomeSuccess})
	p.AuditAccess(context.Background(), AccessKindResource, "file:///a", OutcomeDenied, "not allowed")
	if len(events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(events))
	}
	if events[0].Name != "mcp.tool_call" || events[0].Attributes["outcome"] != OutcomeSuccess {
		t.Errorf("tool audit event wrong: %+v", events[0])
	}
	if events[1].Name != "mcp.resource_access" || events[1].Attributes["outcome"] != OutcomeDenied {
		t.Errorf("access audit event wrong: %+v", events[1])
	}
}
