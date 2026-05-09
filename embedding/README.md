# gokit/embedding

`embedding` owns the canonical embedding abstraction plus a deterministic in-memory adapter for tests.

## Install

```bash
go get github.com/kbukum/gokit/embedding
go get github.com/kbukum/gokit/embedding/inmem
```

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/embedding/inmem"
)

func main() {
	provider := inmem.New(8)

	resp, err := provider.Execute(context.Background(), embedding.EmbedRequest{
		Model: ai.Model{Name: "inmem-embedding", Provider: ai.ProviderCustom},
		Inputs: []embedding.EmbedInput{
			embedding.Text{Text: "native provider shapes keep modules composable"},
		},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(resp.Embeddings[0].Dimensions)
}
```

## When to use

Use `embedding` for canonical vector generation contracts. Provider implementations live here or in the provider-specific module that naturally owns the backend.
