# vectorstore

Vector similarity search store abstraction with explicit backend registration
and an in-memory implementation for gokit.

## Features

- **Store Interface**: Abstraction for vector similarity search
- **InMemoryStore**: Thread-safe in-memory implementation with linear scan search
- **Metrics**: Canonical metric names are `cosine`, `dot`, and `l2`
- **Filtering**: Support for field-based filtering on search queries
- **Metadata Support**: Store arbitrary JSON-serializable metadata alongside vectors

## Usage

### Basic Vector Storage and Search

```go
package main

import (
	"context"
	"fmt"
	"github.com/kbukum/gokit/vectorstore"
)

func main() {
	reg := vectorstore.NewFactoryRegistry()
	if err := vectorstore.RegisterMemory(reg); err != nil {
		panic(err)
	}
	store, err := vectorstore.New(reg, vectorstore.Config{
		Provider: vectorstore.ProviderMemory,
		Metric:   vectorstore.MetricCosine,
	})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	
	// Ensure collection exists
	err := store.EnsureCollection(ctx, "documents", 384)
	if err != nil {
		panic(err)
	}
	
	// Upsert vectors with metadata
	payload1 := vectorstore.NewPointPayload().
		WithField("title", "Document 1").
		WithField("source", "web")
	
	err = store.Upsert(ctx, "documents", "doc1", []float32{
		0.1, 0.2, 0.3, // ... 384 dimensions total
	}, payload1)
	if err != nil {
		panic(err)
	}
	
	// Search for similar vectors
	query := []float32{0.1, 0.2, 0.3, /* ... */}
	results, err := store.Search(ctx, "documents", query, 10, nil)
	if err != nil {
		panic(err)
	}
	
	for _, result := range results {
		fmt.Printf("ID: %s, Score: %.4f, Title: %v\n",
			result.ID,
			result.Score,
			result.Payload.Fields["title"])
	}
}
```

### Searching with Filters

```go
// Search with field filtering
filter := vectorstore.NewSearchFilter().
	MustMatch("source", "web").
	MustMatch("status", "active")

results, err := store.Search(ctx, "documents", query, 10, filter)
if err != nil {
	panic(err)
}
```

### Managing Points

```go
// Delete a point
err := store.Delete(ctx, "documents", "doc1")
if err != nil {
	panic(err)
}

// Update a point (upsert with same ID)
newPayload := vectorstore.NewPointPayload().
	WithField("title", "Updated Document 1")

err = store.Upsert(ctx, "documents", "doc1", newVector, newPayload)
if err != nil {
	panic(err)
}
```

## Store Interface

```go
type Store interface {
	EnsureCollection(ctx context.Context, collection string, dimensions int) error
	Upsert(ctx context.Context, collection, id string, vector []float32, payload *PointPayload) error
	Search(ctx context.Context, collection string, vector []float32, limit int, filter *SearchFilter) ([]SearchResult, error)
	Delete(ctx context.Context, collection, id string) error
}
```

## Data Types

### PointPayload

Stores metadata for each vector point.

```go
payload := vectorstore.NewPointPayload().
	WithField("key1", "value1").
	WithField("count", 42).
	WithField("active", true)
```

Supports any JSON-serializable types: strings, numbers, booleans, objects, arrays.

### SearchResult

Result from a search query.

```go
type SearchResult struct {
	ID      string
	Score   float32      // Cosine similarity score [-1, 1]
	Payload *PointPayload
}
```

### SearchFilter

Optional filtering for search queries.

```go
filter := vectorstore.NewSearchFilter().
	MustMatch("field1", "value1").
	MustMatch("field2", 42)
```

All conditions are AND-ed together. Only exact matches are supported.

## Thread Safety

`InMemoryStore` is thread-safe via `sync.RWMutex`.
Multiple goroutines can safely call methods concurrently.

## Performance Notes

`InMemoryStore` is designed for testing and prototyping:

- Linear scan search: O(n) per query
- No indexing or optimization
- All data stored in memory
- Not suitable for production with large datasets

For production use cases, consider:
- Qdrant for vector databases
- Weaviate for vector search
- Pinecone for managed vector stores
- Elasticsearch with vector search

## Testing

Run tests with:

```bash
go test ./... -race -count=1
```

Run linting with:

```bash
go vet ./...
```

## Examples

### Multi-document RAG

```go
// Store multiple document embeddings
for i, embedding := range embeddings {
	payload := vectorstore.NewPointPayload().
		WithField("doc_id", docIDs[i]).
		WithField("chunk_index", i)
	
	err := store.Upsert(ctx, "rag_docs", fmt.Sprintf("doc_%d", i), 
		embedding, payload)
	if err != nil {
		panic(err)
	}
}

// Search for similar documents
results, err := store.Search(ctx, "rag_docs", queryEmbedding, 5, nil)
```

### Filtered Search

```go
// Store with source metadata
payload := vectorstore.NewPointPayload().
	WithField("source", "web").
	WithField("timestamp", "2024-01-15")

// Search only web documents
filter := vectorstore.NewSearchFilter().
	MustMatch("source", "web")

results, err := store.Search(ctx, "documents", query, 10, filter)
```
