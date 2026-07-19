# gokit/llm

`llm` owns gokit's canonical chat-completion surface: request/response types, canonical stream events, the provider-facing `Dialect` seam, and the adapter that turns provider wire formats into one SDK-free API.

## Install

```bash
go get github.com/kbukum/gokit/llm
go get github.com/kbukum/gokit/llm/providers/ollama
```

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/ollama"
)

func main() {
	ctx := context.Background()

	registry := llm.NewDialectRegistry()
	if err := ollama.Register(registry); err != nil {
		panic(err)
	}

	adapter, err := llm.New(registry, llm.Config{
		Dialect: ollama.DialectName,
		BaseURL: ollama.DefaultBaseURL,
		Model:   "llama3.2",
	})
	if err != nil {
		panic(err)
	}

	resp, err := adapter.Execute(ctx, llm.CompletionRequest{
		Messages: []chat.Message{
			chat.User("Explain why explicit registries are useful."),
		},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(resp.Text())
}
```

## When to use

Use `llm` for chat-style completions, tool calling, and canonical streaming. Use `inference` when you are integrating lower-level serving runtimes such as Triton, vLLM, or TGI.
