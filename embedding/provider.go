package embedding

import (
	"context"
)

// Provider is the interface for generating vector embeddings from text.
type Provider interface {
	// Embed generates an embedding vector for a single text input.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embedding vectors for a batch of text inputs.
	// The order of results corresponds to the order of inputs.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the embedding vectors.
	Dimensions() int
}
