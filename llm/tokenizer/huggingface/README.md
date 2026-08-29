# gokit/llm/tokenizer/huggingface

A Hugging Face `llm.TokenCounter` backed by the pure-Go [`sugarme/tokenizer`](https://github.com/sugarme/tokenizer), for exact token counts against any tokenizer serialized in the Hugging Face `tokenizer.json` format.

The binding is pure Go (no cgo, no native libraries), so it builds and counts on the default build. The tokenizer definition is loaded explicitly from a local file or reader — **never over the network** — and validated with a probe encode at construction, so a malformed definition fails fast.

## Install

```bash
go get github.com/kbukum/gokit/llm/tokenizer/huggingface
```

## Usage

```go
counter, err := huggingface.FromFile("tokenizer.json")
if err != nil {
	return err
}
n, err := counter.Count("hello world") // exact token count
```

From config or a reader:

```go
counter, err := huggingface.NewCounter(huggingface.Config{Path: "tokenizer.json"})
// or
counter, err := huggingface.FromReader(r, 0) // 0 ⇒ DefaultMaxDefinitionBytes
```

The returned counter implements `llm.TokenCounter` and plugs into bench's `metric.TokenStats`.

## Notes

- Reads are bounded by `DefaultMaxDefinitionBytes` (64 MiB) unless overridden via `Config.MaxDefinitionBytes`.
- Text is encoded without special tokens, so counts are pure content tokens.
- `Name()` returns `huggingface:<fingerprint>`, where the fingerprint is a short digest of the definition bytes, so distinct tokenizers are distinguishable.
