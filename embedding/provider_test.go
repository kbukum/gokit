package embedding_test

import (
	"encoding/json"
	"reflect"
	"strings"
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

	// Inputs serialize as a tagged {type, value} discriminator; asset inputs
	// nest a second {type, value} for bytes/url.
	if got := gjsonType(t, data, 0); got != "text" {
		t.Fatalf("inputs[0] type = %q, want text", got)
	}
	if got := gjsonType(t, data, 1); got != "image" {
		t.Fatalf("inputs[1] type = %q, want image", got)
	}

	var got embedding.EmbedRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Model.Name != "m" || got.Model.Provider != ai.ProviderCustom {
		t.Fatalf("model round-trip = %+v", got.Model)
	}
	if got.Options["normalize"] != true {
		t.Fatalf("options round-trip = %+v", got.Options)
	}
	want := []embedding.EmbedInput{
		embedding.Text{Text: "hello"},
		embedding.Image{URL: "https://example/i.png"},
		embedding.Audio{Data: []byte{1, 2}},
		embedding.Video{URL: "https://example/v.mp4"},
	}
	if !reflect.DeepEqual(got.Inputs, want) {
		t.Fatalf("inputs round-trip = %#v, want %#v", got.Inputs, want)
	}
}

// gjsonType extracts inputs[i].type from the serialized request without pulling
// in a JSON path dependency.
func gjsonType(t *testing.T, data []byte, i int) string {
	t.Helper()
	var doc struct {
		Inputs []struct {
			Type string `json:"type"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode inputs: %v", err)
	}
	if i >= len(doc.Inputs) {
		t.Fatalf("input index %d out of range (%d inputs)", i, len(doc.Inputs))
	}
	return doc.Inputs[i].Type
}

func TestEmbedResponseJSONRoundTrip(t *testing.T) {
	t.Parallel()

	resp := embedding.EmbedResponse{
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
	// Array-only carrier (D10): no scalar "embedding" key.
	if strings.Contains(string(data), `"embedding"`) {
		t.Fatalf("response must not emit a scalar embedding key: %s", data)
	}

	var got embedding.EmbedResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Embeddings) != 1 || got.Embeddings[0].Dimensions != 2 || len(got.Embeddings[0].Vector) != 2 {
		t.Fatalf("embeddings round-trip = %+v", got.Embeddings)
	}
	if got.Embeddings[0].Vector[0] != 0.1 || got.Embeddings[0].Vector[1] != 0.2 {
		t.Fatalf("vector round-trip = %v", got.Embeddings[0].Vector)
	}
}
