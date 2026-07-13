package tgi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/inference"
)

func TestPredict_OAICompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"text": "ok", "finish_reason": "length"}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 1},
		})
	}))
	defer srv.Close()

	p, err := New(Config{BaseURL: srv.URL, BearerToken: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := p.Predict(context.Background(), inference.PredictRequest{
		ModelName: "tgi-model",
		Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if resp.Outputs["text"].Text != "ok" {
		t.Errorf("got %q", resp.Outputs["text"].Text)
	}
	if resp.Metadata["finish_reason"] != "length" {
		t.Errorf("got finish_reason %q", resp.Metadata["finish_reason"])
	}
}

func TestDescriptor(t *testing.T) {
	p, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	d := p.Descriptor()
	if d.Name != Kind || !d.Available {
		t.Errorf("unexpected descriptor %+v", d)
	}
}

func TestPredict_WrongInputKind(t *testing.T) {
	p, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Predict(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"prompt": {Kind: inference.KindTensor}},
	})
	if err == nil || !strings.Contains(err.Error(), "must be Text") {
		t.Errorf("expected kind error, got %v", err)
	}
}

func TestPredict_MissingModelName(t *testing.T) {
	p, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Predict(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"prompt": inference.TextValue("hi")},
	})
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Fatalf("expected missing model error, got %v", err)
	}
}

func TestPredictStream_OAICompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"stream":true`) {
			t.Errorf("expected stream flag in body, got %s", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for _, chunk := range []string{
			`{"choices":[{"text":"Hel"}]}`,
			`{"choices":[{"text":"lo"}]}`,
			`{"choices":[{"finish_reason":"stop"}]}`,
			"[DONE]",
		} {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	p, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !p.Descriptor().Capabilities.SupportsStreaming {
		t.Fatal("descriptor should advertise streaming")
	}
	ch, err := p.PredictStream(context.Background(), inference.PredictRequest{
		ModelName: "test",
		Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hello")},
	})
	if err != nil {
		t.Fatalf("PredictStream: %v", err)
	}
	var text strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case ai.TextDelta:
			text.WriteString(e.Text)
		case ai.Error:
			t.Fatalf("stream error: %v", e.Err)
		}
	}
	if text.String() != "Hello" {
		t.Fatalf("streamed text = %q, want %q", text.String(), "Hello")
	}
}

func TestPredictStream_MissingModel(t *testing.T) {
	p, err := New(Config{BaseURL: "http://127.0.0.1:0"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.PredictStream(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"prompt": inference.TextValue("hi")},
	}); err == nil {
		t.Fatal("expected error for missing model")
	}
}
