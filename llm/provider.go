package llm

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// Provider is the interface that LLM backends must implement.
type Provider interface {
	provider.Provider // embeds Name() and IsAvailable()

	// Complete sends a completion request and returns the full response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// CompleteStructured sends a completion request expecting structured JSON
	// output. The schema parameter hints at the desired response structure.
	CompleteStructured(ctx context.Context, req CompletionRequest, schema any) (*CompletionResponse, error)

	// Stream sends a completion request and returns a channel of streamed chunks.
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
}
