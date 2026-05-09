# gokit/mcp MCP 2025-06-18 conformance

This document tracks `gokit/mcp` conformance to the
[Model Context Protocol](https://modelcontextprotocol.io/) revision
**2025-06-18**.

Transports are locked to `stdio` and `streamable_http`; `sse` is not exposed as a peer transport.

| Capability | Status | Notes |
|---|---|---|
| tools/list + tools/call | Present | Registry-backed tool exposure via `gokit/tool`; remote MCP tools wrap as local `tool.Callable`. |
| prompts/list + prompts/get | Present | Static prompt entries registered with `WithPrompt` and served by the MCP Go SDK. |
| resources/list + resources/read | Present | Static resources registered with `WithResource`; SDK owns request dispatch. |
| resource templates | Present | Resource templates registered with `WithResourceTemplate`; URI matching is SDK-owned. |
| cancellation | Partial | Request contexts propagate through SDK sessions/tool calls; explicit compliance vectors remain SDK-limited. |
| progress/logging/pagination/completion | Partial | Native SDK request handlers can be added without protocol forks. |
| structured tool output | Present | Output schema validation runs before MCP result conversion and `tool.Result.Output` maps to `structuredContent`. |
| tool annotations | Present | Derived from `tool.Envelope` and local annotations at the MCP boundary. |
| roots/sampling/elicitation | Partial | Client-side/server-side seams depend on upstream SDK capability exposure. |
| stdio transport | Present | Canonical name `stdio`; SDK stdio transport is used directly. |
| streamable_http transport | Present | Canonical name `streamable_http`; SSE is not a separate transport. |
| Origin validation | Present | Streamable HTTP helper validates configured origins. |
| localhost bind | Present | Security helper validates local binds; SDK Streamable HTTP localhost protection remains enabled by default. |
| payload limits | Present | Server `max_input_bytes`/`max_result_bytes` enforce tool input/result payload policy. |
| OAuth 2.1 + PKCE | Partial | Helper/options seam is composition-owned; this module does not implement a full authorization server. |

Remote MCP servers are tool sources, not skills. Skill manifests live in `gokit/skill`.
