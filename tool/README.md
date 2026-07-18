# gokit/tool

`tool` owns typed callable definitions, JSON Schema input/output, structured results, registries,
batch dispatch, and the executable permission envelope.

## Architecture

```mermaid
flowchart TD
    ToolMod[tool]
    Typed[Tool[I,O]\nHandler]
    Def[Definition\nAnnotations Envelope]
    Callable[Callable]
    Registry[Registry\nCall CallBatch]
    Schema[schema]
    Security[security]
    Agent[agent]
    MCP[mcp]
    App[app-defined tools]

    ToolMod --> Typed
    ToolMod --> Def
    ToolMod --> Callable
    ToolMod --> Registry
    Def --> Schema
    Def --> Security
    Agent --> Registry
    MCP --> Registry
    App --> Typed
```
