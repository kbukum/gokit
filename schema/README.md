# schema

JSON Schema generation from Go types. Wraps [`invopop/jsonschema`](https://github.com/invopop/jsonschema) to produce standard JSON Schema 2020-12 documents from struct tags, exposed as a plain `map[string]any` suitable for tool definitions, MCP, and LLM APIs. Values are validated with [`santhosh-tekuri/jsonschema`](https://github.com/santhosh-tekuri/jsonschema) against draft 2020-12 with asserted formats, bounded depth, node-count, and string-byte limits.

## Install

```bash
go get github.com/kbukum/gokit/schema
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kbukum/gokit/schema"
)

type SearchInput struct {
    Query string `json:"query" jsonschema:"required,description=Search query text"`
    Limit int    `json:"limit" jsonschema:"minimum=1,maximum=100"`
}

func main() {
    s := schema.Generate[SearchInput](schema.WithTitle("SearchInput"))
    fmt.Println(s["type"]) // object

    // Validate a value once, or precompile for reuse.
    res := schema.Validate(s, map[string]any{"query": "golang", "limit": 10})
    fmt.Println(res.Valid)
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Generate[T](opts...)` | Generate a JSON Schema (`JSON`) from a Go type |
| `From(reflectType, opts...)` | Generate a schema from a `reflect.Type` |
| `JSON` (`= map[string]any`) | Plain JSON Schema document |
| `WithTitle()` / `WithDescription()` / `WithDefinitions()` / `WithAdditionalProperties()` | Generation options |
| `Validate(schema, value)` | Validate a value against a schema |
| `Compile()` / `CompileWithLimits()` | Precompile a schema into a reusable `CompiledSchema` |
| `CompiledSchema.Validate(value)` | Validate against a precompiled schema |
| `ValidationLimits` / `DefaultLimits()` / `LimitError` | Depth / node-count / string-byte bounds |
| `GenerateDocument[T]()` / `SchemaDocument` | Generate a schema wrapped in a validated document |
| `ValidateStructuredOutput(schema, value)` | Validate a model's structured output, returning every failure in a `ValidationResult` |
| `ValidationOptions` / `CompileWithOptions()` / `ValidateWithOptions()` | Validation with explicit options and limits |

---

[⬅ Back to main README](../README.md)
