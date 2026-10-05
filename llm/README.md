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

## Bounded streaming

`Adapter.Stream` and `Provider.Stream` emit separate `TextDelta`, `ReasoningDelta`, `ToolUseDelta`, `MessageStart` and `UsageDelta` events. Only `MessageComplete` certifies a complete result. Provider failure, malformed data, premature EOF or quota exhaustion emits `StreamError`, never truncated success. OpenAI-compatible streams wait for `[DONE]` so usage-only tail chunks are preserved.

`CompletionResponse.UsageReported` distinguishes missing usage from reported zero; `UsageDelta` values are cumulative provider snapshots, not amounts to add. Partial updates preserve earlier counters. `Reasoning` remains separate from `Message` text. Model/response identity and the provider stop reason survive assembly. Consumers measuring first content must observe `TextDelta`, not reasoning, tools, metadata or usage.

`Config.Stream` uses HTTP's finite budgets: 10s connect/TLS, 30s headers, 60s first model progress, 30s idle after progress, and 5m total including admission and event delivery. Content, reasoning or tools count as first progress. Later valid chunks renew idle; comments, ping events and partial bytes do not. Caller cancellation and shorter policy deadlines close the upstream stream. There are no streaming retries.

`Config.Stream.MaxFrameBytes` defaults to 1 MiB. SSE counts the complete block with normalized line endings; NDJSON counts payload bytes excluding CR/LF. `Config.StreamLimits` defaults to 8 MiB cumulative output (content, reasoning, metadata and tool deltas), 1 MiB tool arguments and 128 distinct tools. Checks happen before growing assembly. Overflow returns a typed error matching `llm.ErrStreamLimit` (SSE parser limits use the canonical typed decoder error).

One goroutine owns decoding and assembly. A one-slot event channel applies backpressure; cancel the context when stopping consumption. Failure can replace the last undelivered delta with `StreamError` so a full channel cannot strand cleanup or hide the failure. Body closure, reader exit and timer cleanup precede admission release.

Run the scripted upstream proof from the gokit root; no credentials, model download or paid endpoint is needed:

```sh
go test ./llm/providers -run '^TestStreamingIntegration$' -race -shuffle=on -count=1 -timeout=30s
go test ./httpclient ./llm ./llm/providers ./sse -race -shuffle=on -count=1 -timeout=90s
```

The integration uses owned loopback HTTP with 100ms failure budgets, a 1s total call budget and 2s scenario/cleanup watchdogs. It checks success, missing headers/progress, stalls, cancellation, unterminated events, malformed/provider failures, incomplete EOF and cumulative overflow, including exactly one request and released upstream handlers/admission. Boundary tests also cover the default 1 MiB SSE and NDJSON limits. Mirroring these contracts into rskit is separate work.

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
