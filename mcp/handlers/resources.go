package handlers

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
)

// ResourceEntry is a static MCP resource registration.
type ResourceEntry struct {
	Resource *sdkmcp.Resource
	Handler  sdkmcp.ResourceHandler
}

// ResourceTemplateEntry is a static MCP resource-template registration.
type ResourceTemplateEntry struct {
	Template *sdkmcp.ResourceTemplate
	Handler  sdkmcp.ResourceHandler
}

// wrapResourceHandler enforces the resource allow-list keyed on uri
// and audits the outcome around the caller-supplied handler.
func (h *Handler) wrapResourceHandler(uri string, handler sdkmcp.ResourceHandler) sdkmcp.ResourceHandler {
	return func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		if !h.policy.AllowsResource(uri) {
			h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindResource, Target: uri, Outcome: security.OutcomeDenied, Reason: "not in allow-list"})
			return nil, fmt.Errorf("resource not allowed: %s", uri)
		}
		res, err := handler(ctx, req)
		if err != nil {
			h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindResource, Target: uri, Outcome: security.OutcomeToolError, Reason: err.Error()})
			return nil, err
		}
		h.policy.AuditAccess(ctx, security.AccessAuditEvent{Kind: security.AccessKindResource, Target: uri, Outcome: security.OutcomeSuccess, Reason: ""})
		return res, nil
	}
}
