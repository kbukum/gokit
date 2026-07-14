// Package handlers holds the server-side MCP protocol handlers: inbound feature
// registration (tools, prompts, resources) behind the fail-closed hardening
// chain, and the outbound server-to-client helpers (sampling, elicitation,
// roots, logging, progress) that treat model output and elicited content as
// untrusted.
//
// A Handler bundles the injected security.Policy, the backing tool.Registry,
// and the exposed-name prefix so each protocol feature file hangs its logic off
// one dependency set. The parent mcp package owns construction and public API;
// it drives registration during NewServer and delegates the outbound helpers to
// the Handler.
package handlers
