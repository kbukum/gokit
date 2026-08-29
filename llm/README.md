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

## Token counting

`llm` owns gokit's canonical token-counting concern through the `TokenCounter` port:

```go
type TokenCounter interface {
	Name() string
	Count(text string) (int, error)
}
```

`HeuristicTokenCounter` is the dependency-free default. It shares the `chars/4` approximation with `ai/chat` (via `ai/chat.ApproxTokens`), so estimates stay consistent everywhere and no divergent rule is introduced:

```go
counter := llm.HeuristicTokenCounter{}
n, _ := counter.Count("hello world") // approximate; the heuristic never errors
```

`Count` is fallible: an exact tokenizer can surface an encode error at call time, so it returns `(int, error)` rather than substituting a success-shaped count. The heuristic never fails and always returns a `nil` error.

For exact counts, inject a contrib counter backed by a real tokenizer. Each lives in its own sub-module so its tokenizer dependency stays out of core:

- [`llm/tokenizer/tiktoken`](tokenizer/tiktoken) — OpenAI BPE via `tiktoken-go`, offline embedded vocab.
- [`llm/tokenizer/huggingface`](tokenizer/huggingface) — any Hugging Face `tokenizer.json`, pure-Go via `sugarme/tokenizer`.

```go
counter, err := tiktoken.New(tiktoken.Cl100kBase)
// or: counter, err := huggingface.FromFile("tokenizer.json")
```

Any `TokenCounter` plugs into bench's `metric.TokenStats` metric for per-prediction token usage.
