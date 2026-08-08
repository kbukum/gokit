# gokit/mcp

`mcp` is a hardened, protocol-shaped [Model Context Protocol](https://modelcontextprotocol.io/) server and client backed by gokit's `tool.Registry`. The official [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk) owns the protocol wire (tools, prompts, resources + templates, subscribe, roots, sampling, elicitation, logging, progress, cancellation) over the **stdio** and **Streamable HTTP** transports; this package is the typed, fail-closed facade on top of it.

## Server hardening chain

Every `tools/call` runs a fail-closed chain — any step short-circuits to an audited MCP error result:

```
allow-list → input-size limit → schema validation → authorization
  → registry dispatch (destructive-tool human gate) → result-size limit
  → output schema validation → audit
```

Prompts and resources share the capability allow-list and audit. Under MCP 2026-07-28 (SEP-2322), server-initiated sampling, elicitation, and roots requests must be issued as multi round-trip *input requests* from inside a tool: register one with `Server.AddInteractiveTool`, return an `InputRequests` map to ask the client for input, and read the responses back with `Server.SampleResponse` / `ElicitResponse` / `RootsResponse`, which treat model output and elicited content as untrusted (size-limited and audited). The standalone `Sample`, `Elicit`, and `ListRoots` helpers remain for clients negotiating an earlier protocol version (the SEP-2577 deprecation window) and still guard against nil sessions; `Log` and `NotifyProgress` send notifications.

Untrusted client/model payloads are carried as `json.RawMessage`; documented JSON-Schema is `schema.JSON`. No `interface{}`/`any` on exported surfaces beyond those documented opaque types.

## Transports

- **stdio** — `Server.ServeStdio(ctx)` for local, single-client integrations (IDEs, agents).
- **Streamable HTTP** — `Server.StreamableHTTPHandler(cfg, token)` wraps the SDK handler with Origin cross-origin protection and optional constant-time bearer-token auth. Localhost (loopback) protection is on by default.

## Architecture

```mermaid
flowchart TD
    MCP[mcp]
    Server[Server\nlocal tools -> hardened MCP]
    Client[Connect\nremote MCP -> callables]
    Stdio[ServeStdio]
    HTTP[StreamableHTTPHandler\nOrigin + bearer auth]
    S2C[server->client\ninteractive MRTR + Sample/Elicit/ListRoots/Log/NotifyProgress]
    SkillAdapter[skill adapter\nallow-list pinned]
    ToolReg[tool.Registry\nHITL destructive gate]
    Security[allow-list / authz / size limits / audit]
    Obs[observability]
    SDK[go-sdk/mcp]

    MCP --> Server
    MCP --> Client
    MCP --> SkillAdapter
    Server --> Stdio
    Server --> HTTP
    Server --> S2C
    Server --> Security
    Server --> ToolReg
    Client --> ToolReg
    Server --> Obs
    Client --> Obs
    Stdio --> SDK
    HTTP --> SDK
    S2C --> SDK
```

See [`CONFORMANCE.md`](CONFORMANCE.md) for the full 2025-06-18 capability matrix. Remote MCP servers are tool sources, not skills; skill manifests live in `gokit/skill`.
