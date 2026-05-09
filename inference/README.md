# gokit/inference

`inference` is the model-serving runtime layer. It adapts serving backends into one `Predict` surface with typed values.

## Install

```bash
go get github.com/kbukum/gokit/inference
go get github.com/kbukum/gokit/inference/vllm
```

## Adapters

| Adapter | Protocol | Streaming | Status |
| --- | --- | --- | --- |
| `echo` | local test adapter | No | ✅ Implemented |
| `triton` | KServe v2 HTTP | No | ✅ Implemented |
| `vllm` | OpenAI-compatible `/v1/completions` | No | ✅ Implemented |
| `tgi` | OpenAI-compatible `/v1/completions` | No | ✅ Implemented |

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/inference/vllm"
)

func main() {
	provider, err := vllm.New(vllm.Config{
		BaseURL: "http://localhost:8000",
	})
	if err != nil {
		panic(err)
	}

	resp, err := provider.Predict(context.Background(), inference.PredictRequest{
		ModelName: "llama3",
		Inputs: map[string]inference.Value{
			"prompt": inference.TextValue("Write a short deployment note."),
		},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(resp.Outputs["text"].Text)
}
```

## When to use

Use `inference` for model-serving backends and typed runtime I/O. If you want chat semantics, tool use, or canonical message streaming, stay in `llm`.
