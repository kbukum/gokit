# gokit/llm/tokenizer/tiktoken

An OpenAI BPE `llm.TokenCounter` backed by [`tiktoken-go`](https://github.com/pkoukk/tiktoken-go), for exact token counts against OpenAI encodings.

The BPE ranks are loaded from an offline, embedded vocab ([`tiktoken-go-loader`](https://github.com/pkoukk/tiktoken-go-loader)), so counting is fully deterministic with **no network access** and no import-time side effects.

## Install

```bash
go get github.com/kbukum/gokit/llm/tokenizer/tiktoken
```

## Usage

```go
counter, err := tiktoken.New(tiktoken.Cl100kBase)
if err != nil {
	return err
}
n, err := counter.Count("hello world") // exact BPE token count
```

Or construct from config:

```go
counter, err := tiktoken.NewCounter(tiktoken.Config{Encoding: "o200k_base"})
```

The returned counter implements `llm.TokenCounter` and plugs into bench's `metric.TokenStats`.

## Supported encodings

| Encoding | Constant | Models |
|---|---|---|
| `o200k_base` | `tiktoken.O200kBase` | GPT-4o, o-series |
| `cl100k_base` | `tiktoken.Cl100kBase` | GPT-3.5-turbo, GPT-4 |
| `p50k_base` | `tiktoken.P50kBase` | older Codex / text-davinci |
| `r50k_base` | `tiktoken.R50kBase` | GPT-3 (davinci) |

`Name()` returns `tiktoken:<encoding>`.
