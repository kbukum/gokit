package vllm

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
		if r.URL.Path != "/v1/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"prompt":"hello"`) {
			t.Errorf("expected prompt in body, got %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"text": " world", "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
		})
	}))
	defer srv.Close()

	p, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resp, err := p.Predict(context.Background(), inference.PredictRequest{
		ModelName: "test",
		Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hello")},
	})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if resp.Outputs["text"].Text != " world" {
		t.Errorf("got text %q", resp.Outputs["text"].Text)
	}
	if resp.Usage.OutputTokens != 2 {
		t.Errorf("got output tokens %d", resp.Usage.OutputTokens)
	}
	if resp.Status != inference.StatusSuccess {
		t.Errorf("got status %v", resp.Status)
	}
}

func TestDescriptor(t *testing.T) {
	p, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d := p.Descriptor()
	if d.Name != Kind || !d.Available {
		t.Errorf("unexpected descriptor %+v", d)
	}
}

func TestPredict_MissingPrompt(t *testing.T) {
	p, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Predict(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{},
	})
	if err == nil || !strings.Contains(err.Error(), "missing required input") {
		t.Errorf("expected missing-input error, got %v", err)
	}
}

func TestPredict_MissingModelName(t *testing.T) {
	p, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Predict(context.Background(), inference.PredictRequest{
		Inputs: map[string]inference.Value{"prompt": inference.TextValue("hello")},
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
