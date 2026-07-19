package embedding_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/embedding"
)

// sealed EmbedInput implementations must satisfy the interface at compile time.
var (
	_ embedding.EmbedInput = embedding.Text{}
	_ embedding.EmbedInput = embedding.Image{}
	_ embedding.EmbedInput = embedding.Audio{}
	_ embedding.EmbedInput = embedding.Video{}
)

func TestEmbedRequestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	req := embedding.EmbedRequest{
		Model: ai.Model{Name: "m", Provider: ai.ProviderCustom},
		Inputs: []embedding.EmbedInput{
			embedding.Text{Text: "hello"},
			embedding.Image{URL: "https://example/i.png"},
			embedding.Audio{Data: []byte{1, 2}},
			embedding.Video{URL: "https://example/v.mp4"},
		},
		Options: map[string]any{"normalize": true},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Inputs is a sealed interface; a decoder cannot reconstruct the concrete
	// types without a discriminator, so assert the scalar fields survive instead.
	var raw struct {
		Model   ai.Model       `json:"model"`
		Options map[string]any `json:"options"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw.Model.Name != "m" || raw.Model.Provider != ai.ProviderCustom {
		t.Fatalf("model round-trip = %+v", raw.Model)
	}
	if raw.Options["normalize"] != true {
		t.Fatalf("options round-trip = %+v", raw.Options)
	}
}

func TestEmbedResponseJSONRoundTrip(t *testing.T) {
	t.Parallel()

	resp := embedding.EmbedResponse{
		Embedding: embedding.Embedding{Vector: []float32{0.1, 0.2}, Dimensions: 2, Index: 0},
		Embeddings: []embedding.Embedding{
			{Vector: []float32{0.1, 0.2}, Dimensions: 2, Index: 0},
		},
		Model: ai.Model{Name: "m"},
		Usage: ai.Usage{},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got embedding.EmbedResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Embedding.Dimensions != 2 || len(got.Embedding.Vector) != 2 {
		t.Fatalf("embedding round-trip = %+v", got.Embedding)
	}
	if got.Embedding.Vector[0] != 0.1 || got.Embedding.Vector[1] != 0.2 {
		t.Fatalf("vector round-trip = %v", got.Embedding.Vector)
	}
}
