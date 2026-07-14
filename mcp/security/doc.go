// Package security holds the hardening primitives for the MCP server: the
// injected [Policy] that gates every tool, prompt, and resource access
// (capability allow-list, authorization, payload/result size limits, audit,
// and output validation), plus the HTTP-boundary guards (Origin validation and
// bearer-token authentication).
//
// The primitives are transport- and SDK-agnostic: they operate on gokit tool
// and authz/observability types and on net/http, never on the MCP wire types.
// The parent mcp package composes a Policy from its ServerOptions and drives
// these guards from its handlers.
package security
