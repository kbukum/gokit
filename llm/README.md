# llm

LLM provider framework with completion, structured output, streaming, and provider registry. Ships with an Ollama implementation.

## Install

```bash
go get github.com/skillsenselab/gokit/llm@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/llm"
    "github.com/skillsenselab/gokit/llm/ollama"
)

// Create an Ollama provider
p := ollama.NewProvider(ollama.Config{BaseURL: "http://localhost:11434", Model: "llama3"})

resp, _ := p.Complete(ctx, llm.CompletionRequest{
    Messages:     []llm.Message{{Role: "user", Content: "Summarize this text"}},
    SystemPrompt: "You are a helpful assistant.",
})
fmt.Println(resp.Content, resp.Usage.TotalTokens)
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Provider` | Interface — `Complete`, `CompleteStructured`, `Stream` |
| `CompletionRequest` | Model, Messages, Temperature, MaxTokens, SystemPrompt |
| `CompletionResponse` | Content, Model, Usage |
| `StreamChunk` | Content, Done, Err |
| `Message` | Role, Content |
| `NewRegistry()` | Create a provider registry |

### `llm/ollama`

| Symbol | Description |
|---|---|
| `NewProvider(cfg)` | Create an Ollama provider |
| `Factory()` | Provider factory for registry registration |
| `Config` | BaseURL, Model, Temperature, Timeout |

---

[← Back to main gokit README](../README.md)
