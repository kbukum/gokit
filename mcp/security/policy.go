package security

import (
	"context"
	"encoding/json"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/observability"
)

// Policy is the injected, fail-closed hardening configuration applied to every gated MCP access. A zero Policy exposes everything and enforces no limits, authorization, or audit; each field opts a concern in. It is assembled by the parent mcp package from its ServerOptions and consulted by the tool, prompt, and resource handlers.
type Policy struct {
	// AllowedTools, AllowedPrompts, and AllowedResources are capability allow-lists. An empty set exposes everything of that kind; a populated set fails closed, exposing only the named members.
	AllowedTools     map[string]struct{}
	AllowedPrompts   map[string]struct{}
	AllowedResources map[string]struct{}

	// Decider authorizes every tools/call before execution. A nil Decider allows all calls.
	Decider authz.Decider
	// Auditor records every gated access outcome. A nil Auditor disables audit.
	Auditor observability.Auditor

	// MaxInputBytes rejects tool-call arguments larger than the limit. Zero means unlimited.
	MaxInputBytes int
	// MaxResultBytes rejects tool results (and sampled/elicited content) larger than the limit. Zero means unlimited.
	MaxResultBytes int
}

// ToSet builds a membership set from names, the canonical representation for the Policy allow-lists.
func ToSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

func allows(set map[string]struct{}, name string) bool {
	if len(set) == 0 {
		return true
	}
	_, ok := set[name]
	return ok
}

// AllowsTool reports whether name is exposed by the tool allow-list.
func (p *Policy) AllowsTool(name string) bool { return allows(p.AllowedTools, name) }

// AllowsPrompt reports whether name is exposed by the prompt allow-list.
func (p *Policy) AllowsPrompt(name string) bool { return allows(p.AllowedPrompts, name) }

// AllowsResource reports whether uri is exposed by the resource allow-list.
func (p *Policy) AllowsResource(uri string) bool { return allows(p.AllowedResources, uri) }

// InputTooLarge reports whether input exceeds the configured input size limit.
func (p *Policy) InputTooLarge(input json.RawMessage) bool {
	return p.MaxInputBytes > 0 && len(input) > p.MaxInputBytes
}

// ResultTooLarge reports whether a serialized result of size bytes exceeds the configured result size limit.
func (p *Policy) ResultTooLarge(size int) bool {
	return p.MaxResultBytes > 0 && size > p.MaxResultBytes
}

// Authorize consults the configured Decider for a tools/call. A nil Decider allows the call.
func (p *Policy) Authorize(ctx context.Context, req ToolAuthorizationRequest) (authz.Decision, error) {
	if p.Decider == nil {
		return authz.Decision{Allowed: true, Reason: "no_decider"}, nil
	}
	return p.Decider.Decide(ctx, authzRequest(req))
}

func authzRequest(req ToolAuthorizationRequest) authz.Request {
	ctx := authz.Attributes{"mcp_name": req.MCPName}
	if len(req.Arguments) > 0 {
		ctx["arguments"] = string(req.Arguments)
	}
	return authz.Request{
		Resource: authz.Resource{Type: "tool", ID: req.ToolName},
		Action:   authz.ActionToolInvoke,
		Context:  ctx,
	}
}

// DeniedMessage formats the client-facing message for a denied tool call, appending reason when present.
func DeniedMessage(reason string) string {
	if reason == "" {
		return "tool call denied"
	}
	return "tool call denied: " + reason
}
