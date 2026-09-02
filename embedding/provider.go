package embedding

import (
	"context"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/provider"
)

// Provider generates vector embeddings for text and multimodal inputs.
//
// Per locked decision D7 (NATIVE EMBED), Provider natively embeds [provider.RequestResponse]
// so any embedding provider drops into dag / pipeline / chain / worker consumers without a bridge.
// The single-request method IS Execute (the canonical RR method);
// EmbedBatch is the batched extension.
//
// Required methods (by transitive embedding):
//   - Name() string                                                  // provider.Provider
//   - IsAvailable(ctx context.Context) bool                          // provider.Provider
//   - Execute(ctx, EmbedRequest) (EmbedResponse, error)              // RequestResponse
//   - EmbedBatch(ctx, []EmbedRequest) ([]EmbedResponse, error)
type Provider interface {
	provider.RequestResponse[EmbedRequest, EmbedResponse]
	EmbedBatch(ctx context.Context, reqs []EmbedRequest) ([]EmbedResponse, error)
}

// EmbedRequest carries embedding inputs and provider-specific options.
type EmbedRequest struct {
	Model   ai.Model       `json:"model"`
	Inputs  []EmbedInput   `json:"inputs"`
	Options map[string]any `json:"options,omitempty"`
}

// EmbedResponse carries normalized embeddings, model echo, and usage.
// Embeddings is the single vector carrier: one entry per input, in input order (D10, array-only).
type EmbedResponse struct {
	Embeddings []Embedding `json:"embeddings"`
	Model      ai.Model    `json:"model"`
	Usage      ai.Usage    `json:"usage,omitzero"`
}

// Embedding is a normalized vector with its request index.
type Embedding struct {
	Vector     []float32 `json:"vector"`
	Dimensions int       `json:"dimensions"`
	Index      int       `json:"index"`
}
