# gokit/mcp MCP 2025-06-18 conformance

This document tracks `gokit/mcp` conformance to the [Model Context Protocol](https://modelcontextprotocol.io/) revision **2025-06-18**.

Transports are locked to `stdio` and `streamable_http`; `sse` is not exposed as a peer transport.

| Capability | Status | Notes |
|---|---|---|
| tools/list + tools/call | Present | Registry-backed tool exposure via `gokit/tool` behind a fail-closed hardening chain (allow-list → input-size → schema-validate → authz → registry HITL gate → result-size → output-validate → audit); remote MCP tools wrap as local `tool.Callable`. |
| prompts/list + prompts/get | Present | Static prompt entries registered with `WithPrompt`; gated by the prompt allow-list and audited. |
| resources/list + resources/read | Present | Static resources registered with `WithResource`; gated by the resource allow-list and audited. |
| resource templates | Present | Resource templates registered with `WithResourceTemplate`; URI matching is SDK-owned, allow-list keyed on the template URI. |
| resources/subscribe | Present | `WithSubscribeHandler` wires subscribe/unsubscribe and advertises the capability. |
| roots | Present | `Server.ListRoots` is a typed server→client helper (nil-session guard + audit); `WithRootsListChangedHandler` observes client roots changes. |
| sampling | Present | `Server.Sample` treats model output as untrusted: result-size limit + audit; nil-session guard. |
| elicitation | Present | `Server.Elicit` treats elicited content as untrusted: result-size limit + audit; nil-session guard. |
| cancellation | Present | `context.Context` propagates through every tool call and server→client helper; a canceled context aborts the call and surfaces as an error result. |
| progress | Present | `Server.NotifyProgress` sends progress notifications; `WithProgressHandler` observes client progress. |
| logging | Present | `Server.Log` sends `logging/message` notifications over the session. |
| pagination/completion | Partial | Native SDK request handlers can be added without protocol forks. |
| structured tool output | Present | Output schema validation runs before MCP result conversion and `tool.Result.Output` maps to `structuredContent`. |
| tool annotations | Present | Derived from `tool.Envelope` and local annotations at the MCP boundary. |
| destructive-tool gate | Present | Destructive tools are human-gated by the `tool.Registry` HITL flow; without an approver the call fails closed. |
| stdio transport | Present | Canonical name `stdio`; `Server.ServeStdio` uses the SDK stdio transport directly. |
| streamable_http transport | Present | Canonical name `streamable_http`; SSE is not a separate transport. |
| Origin validation | Present | `NewStreamableHTTPOptions` validates and normalizes configured origins (rejects paths/queries/fragments/credentials/non-http/opaque) and preloads `http.CrossOriginProtection`. |
| localhost bind | Present | SDK Streamable HTTP localhost protection stays enabled by default (`DisableLocalhostProtection` opt-in). |
| HTTP bearer auth | Present | `StreamableHTTPHandler` optionally wraps the handler with constant-time bearer-token auth (token in header only; an empty token fails closed with `ErrEmptyBearerToken`). |
| payload limits | Present | `WithMaxInputBytes`/`WithMaxResultBytes` enforce tool input/result payload policy; sampling/elicitation content is size-limited too. |
| OAuth 2.1 + PKCE | Partial | Helper/options seam is composition-owned; this module does not implement a full authorization server. |

Remote MCP servers are tool sources, not skills. Skill manifests live in `gokit/skill`.
