package handlers

import (
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/mcp/security"
	"github.com/kbukum/gokit/tool"
)

// Handler wires a tool.Registry and a security.Policy onto an SDK server. It carries the shared dependencies for every protocol feature so the individual handlers do not thread them through call signatures.
type Handler struct {
	sdk      *sdkmcp.Server
	registry *tool.Registry
	policy   *security.Policy
	prefix   string
}

// New builds a Handler bound to the SDK server, backing registry, hardening policy, and exposed-name prefix.
func New(sdk *sdkmcp.Server, registry *tool.Registry, policy *security.Policy, prefix string) *Handler {
	return &Handler{sdk: sdk, registry: registry, policy: policy, prefix: prefix}
}
