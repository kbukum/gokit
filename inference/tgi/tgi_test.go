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
	"github.com/kbukum/gokit/component"
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

func TestFactoryRegisterLifecycleAndExecute(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"text":"ok"}]}`))
	}))
	defer server.Close()

	reg := inference.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	runtime, err := reg.Build(Kind, json.RawMessage(`{"base_url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := Factory(json.RawMessage(`{`)); err == nil {
		t.Fatal("expected malformed config error")
	}
	p, ok := runtime.(*Provider)
	if !ok {
		t.Fatalf("runtime type = %T", runtime)
	}
	if p.Name() != Kind || !p.IsAvailable(context.Background()) {
		t.Fatalf("provider state: name=%q available=%v", p.Name(), p.IsAvailable(context.Background()))
	}
	if health := p.Health(context.Background()); health.Status != component.StatusDegraded {
		t.Fatalf("initial health = %+v", health)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if health := p.Health(context.Background()); health.Status != component.StatusHealthy || health.Message != "ready" {
		t.Fatalf("started health = %+v", health)
	}
	resp, err := p.Execute(context.Background(), inference.PredictRequest{
		ModelName: "demo",
		Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Outputs["text"].Text != "ok" {
		t.Fatalf("Execute response = %+v", resp)
	}
	if health := p.Health(context.Background()); health.Status != component.StatusHealthy || !strings.Contains(health.Message, "last_call=") {
		t.Fatalf("post-call health = %+v", health)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
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

func TestPredictErrorResponses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "status", status: http.StatusBadGateway, body: "upstream unavailable", wantErr: "HTTP 502"},
		{name: "malformed json", status: http.StatusOK, body: `{`, wantErr: "decode response"},
		{name: "no choices", status: http.StatusOK, body: `{"choices":[]}`, wantErr: "no choices"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			p, err := New(Config{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = p.Predict(context.Background(), inference.PredictRequest{
				ModelName: "demo",
				Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestPredictStreamErrorsAndCancel(t *testing.T) {
	t.Parallel()

	t.Run("http status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "stream denied", http.StatusUnauthorized)
		}))
		defer server.Close()
		p, err := New(Config{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		_, err = p.PredictStream(context.Background(), inference.PredictRequest{
			ModelName: "demo",
			Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
		})
		if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("non sse", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()
		p, err := New(Config{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		_, err = p.PredictStream(context.Background(), inference.PredictRequest{
			ModelName: "demo",
			Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
		})
		if err == nil || !strings.Contains(err.Error(), "SSE stream") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cancel mid stream", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`data: {"choices":[{"text":"first"}]}` + "\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		}))
		defer server.Close()
		p, err := New(Config{BaseURL: server.URL})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := p.PredictStream(ctx, inference.PredictRequest{
			ModelName: "demo",
			Inputs:    map[string]inference.Value{"prompt": inference.TextValue("hi")},
		})
		if err != nil {
			t.Fatalf("PredictStream: %v", err)
		}
		ev := <-ch
		if delta, ok := ev.(ai.TextDelta); !ok || delta.Text != "first" {
			t.Fatalf("first event = %#v", ev)
		}
		cancel()
		for range ch {
		}
	})
}

func TestExecRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	p, err := New(Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.exec(context.Background(), http.MethodGet, "/v1/completions", nil); err == nil || !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("error = %v", err)
	}
}
