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

// EmbedInput is the sealed interface for embedding inputs.
type EmbedInput interface {
	embedInput()
}

// Text is a text embedding input.
type Text struct {
	Text string `json:"text"`
}

func (Text) embedInput() {}

// Image is an image embedding input, either inline bytes or a URL.
type Image struct {
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (Image) embedInput() {}

// Audio is an audio embedding input, either inline bytes or a URL.
type Audio struct {
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (Audio) embedInput() {}

// Video is a video embedding input, either inline bytes or a URL.
type Video struct {
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (Video) embedInput() {}

// EmbedResponse carries normalized embeddings, model echo, and usage.
type EmbedResponse struct {
	Embedding  Embedding   `json:"embedding"`
	Embeddings []Embedding `json:"embeddings,omitempty"`
	Model      ai.Model    `json:"model"`
	Usage      ai.Usage    `json:"usage,omitempty"`
}

// Embedding is a normalized vector with its request index.
type Embedding struct {
	Vector     []float32 `json:"vector"`
	Dimensions int       `json:"dimensions"`
	Index      int       `json:"index"`
}
