# embedding

Embedding provider abstraction and OpenAI-compatible implementation for gokit.

## Features

- **Provider Interface**: Abstraction for text embedding generation
- **OpenAI-Compatible Provider**: Works with OpenAI, Azure OpenAI, local llama.cpp, vLLM, or any server exposing `/v1/embeddings`
- **Vector Utilities**: Distance and aggregation functions:
  - Cosine similarity
  - Euclidean distance
  - Dot product
  - Mean pooling
  - Max pooling
  - Vector normalization

## Usage

### Basic Embedding

```go
package main

import (
	"context"
	"fmt"
	"github.com/kbukum/gokit/embedding"
)

func main() {
	config := embedding.OpenAIConfig{
		Endpoint:   "https://api.openai.com",
		APIKey:     "sk-...",
		Model:      "text-embedding-3-small",
		Dimensions: 1536,
	}
	
	provider := embedding.NewOpenAIProvider(config)
	
	ctx := context.Background()
	vec, err := provider.Embed(ctx, "Hello, world!")
	if err != nil {
		panic(err)
	}
	
	fmt.Printf("Embedding dimensions: %d\n", provider.Dimensions())
	fmt.Printf("First 5 values: %v\n", vec[:5])
}
```

### Batch Embedding

```go
texts := []string{
	"First document",
	"Second document",
	"Third document",
}

embeddings, err := provider.EmbedBatch(ctx, texts)
if err != nil {
	panic(err)
}

fmt.Printf("Generated %d embeddings\n", len(embeddings))
```

### Vector Operations

```go
// Cosine similarity
sim, err := embedding.CosineSimilarity(vec1, vec2)
if err != nil {
	panic(err)
}
fmt.Printf("Similarity: %.4f\n", sim)

// Mean pooling
mean, err := embedding.MeanPooling([][]float32{vec1, vec2})
if err != nil {
	panic(err)
}

// Normalize vector
normalized, err := embedding.Normalize(vec)
if err != nil {
	panic(err)
}
```

## Provider Interface

```go
type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}
```

## Configuration

### OpenAIConfig

- **Endpoint**: Base URL for the API (default: "https://api.openai.com")
- **APIKey**: API key for authentication (can be empty)
- **Model**: Model name (default: "text-embedding-3-small")
- **Dimensions**: Expected embedding dimensions (default: 1536)

## Vector Functions

All vector functions return an error if vectors have mismatched dimensions.

- `CosineSimilarity(a, b []float32) (float32, error)` - Returns value in [-1.0, 1.0]
- `EuclideanDistance(a, b []float32) (float32, error)` - Returns distance
- `DotProduct(a, b []float32) (float32, error)` - Returns dot product
- `MeanPooling(vectors [][]float32) ([]float32, error)` - Element-wise average
- `MaxPooling(vectors [][]float32) ([]float32, error)` - Element-wise maximum
- `Normalize(v []float32) ([]float32, error)` - Returns unit vector

## Testing

Run tests with:

```bash
go test ./... -race -count=1
```

Run linting with:

```bash
go vet ./...
```
