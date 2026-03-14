# llm

Config-driven LLM adapter built on gokit's HTTP/REST foundation. Works with any LLM provider (Ollama, OpenAI, Anthropic, etc.) via the Dialect pattern — similar to `database/sql` drivers.

## Install

```bash
go get github.com/kbukum/gokit/llm@latest
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/kbukum/gokit/llm"
    _ "github.com/your-org/llm-ollama" // registers "ollama" dialect
)

func main() {
    adapter, _ := llm.New(llm.Config{
        Dialect: "ollama",
        BaseURL: "http://localhost:11434",
        Model:   "qwen2.5:1.5b",
    })

    resp, _ := adapter.Execute(context.Background(), llm.CompletionRequest{
        Messages: []llm.Message{{Role: "user", Content: "Hello!"}},
    })
    fmt.Println(resp.Content)
}
```

## Helpers

```go
// Simple text completion
text, err := llm.Complete(ctx, adapter, "You are a helper.", "What is Go?")

// Structured JSON output (auto-unmarshals)
var result MyStruct
err := llm.CompleteStructured(ctx, adapter, systemPrompt, userPrompt, &result)
```

## Streaming

```go
ch, err := adapter.Stream(ctx, llm.CompletionRequest{
    Messages: []llm.Message{{Role: "user", Content: "Tell me a story"}},
})
for chunk := range ch {
    if chunk.Err != nil { break }
    fmt.Print(chunk.Content)
}
```

## Sub-Packages

### `llm/explain` — Structured Explanation Generation

Takes analysis signals and produces human-readable explanations via an LLM:

```go
import "github.com/kbukum/gokit/llm/explain"

signals := []explain.Signal{
    {Name: "frequency_score", Value: 0.92, Label: "Frequency analysis"},
    {Name: "metadata_score", Value: 0.15, Label: "Metadata check"},
}

exp, err := explain.Generate(ctx, adapter, explain.Request{
    Signals: signals,
})
fmt.Println(exp.Summary)      // "Content is likely AI-generated..."
fmt.Println(exp.KeyFactors)   // ["frequency_score"]
fmt.Println(exp.Confidence)   // 0.87
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Adapter` | LLM client implementing `provider.RequestResponse` and `provider.Streamable` |
| `Config` | YAML/JSON-serializable adapter configuration |
| `Dialect` | Interface for provider-specific HTTP mapping |
| `CompletionRequest` / `CompletionResponse` | Universal input/output types |
| `StreamChunk` | Single piece of a streamed response |
| `Message` | Chat message with Role and Content |
| `Usage` | Token consumption report |
| `Complete()` | Convenience: system + user prompt → text |
| `CompleteStructured()` | Convenience: prompt → parsed JSON struct |
| `RegisterDialect()` / `GetDialect()` | Global dialect registry |
| `explain.Generate()` | Signals → structured LLM explanation |
| `explain.Signal` | Analysis signal (name, value, label) |
| `explain.Explanation` | Structured output with reasoning steps |

---

[⬅ Back to main README](../README.md)
