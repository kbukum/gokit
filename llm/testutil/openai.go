package testutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const (
	// MaxOpenAIRequestBytes bounds one chat request body.
	MaxOpenAIRequestBytes = 2 << 20
	// DefaultMaxModels bounds how many distinct model names an [OpenAIServer] counts.
	DefaultMaxModels = 10_000
	// EchoPrefix starts every [OpenAIServer] answer.
	EchoPrefix = "echo: "
	// FailModelPrefix names models whose calls are counted and then answered with 502 Bad Gateway.
	FailModelPrefix = "fail-"
)

// OpenAIServer is a deterministic OpenAI-compatible chat model for tests. It answers POST /v1/chat/completions with
// "echo: " followed by the last user message and reports word-count usage, streamed as server-sent events when the
// request asks for a stream. It counts calls per model, served over GET /calls?model=<name> and [OpenAIServer.Calls].
// Models named with [FailModelPrefix] are counted and then fail with 502. It requires a bearer token but never reads
// or stores its value. It is safe for concurrent use; serve it with httptest or an http.Server.
type OpenAIServer struct {
	mux       *http.ServeMux
	maxModels int

	mu    sync.Mutex
	calls map[string]int
}

// OpenAIServerOption configures an [OpenAIServer].
type OpenAIServerOption func(*OpenAIServer)

// WithMaxModels bounds how many distinct model names are counted. Calls to a new model past the bound answer 503.
// A non-positive value keeps [DefaultMaxModels].
func WithMaxModels(n int) OpenAIServerOption {
	return func(s *OpenAIServer) {
		if n > 0 {
			s.maxModels = n
		}
	}
}

// NewOpenAIServer returns a fake chat model with no recorded calls.
func NewOpenAIServer(opts ...OpenAIServerOption) *OpenAIServer {
	s := &OpenAIServer{maxModels: DefaultMaxModels, calls: map[string]int{}}
	for _, opt := range opts {
		opt(s)
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /v1/chat/completions", s.chat)
	s.mux.HandleFunc("GET /calls", s.count)
	return s
}

// ServeHTTP serves the chat and call-count routes.
func (s *OpenAIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Calls reports how many accepted requests named model.
func (s *OpenAIServer) Calls(model string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[model]
}

type chatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (s *OpenAIServer) chat(w http.ResponseWriter, r *http.Request) {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); !ok || token == "" {
		http.Error(w, "bearer token required", http.StatusUnauthorized)
		return
	}
	var req chatRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxOpenAIRequestBytes))
	if err := dec.Decode(&req); err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	input, ok := lastUser(req)
	if !ok || req.Model == "" {
		http.Error(w, "a model and a user message are required", http.StatusBadRequest)
		return
	}
	if !s.record(req.Model) {
		http.Error(w, "too many models", http.StatusServiceUnavailable)
		return
	}
	if strings.HasPrefix(req.Model, FailModelPrefix) {
		http.Error(w, "model failed", http.StatusBadGateway)
		return
	}

	prompt, completion := words(input), words(EchoPrefix+input)
	usage := map[string]int{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": prompt + completion}
	w.Header().Set("Cache-Control", "no-store")
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion", "model": req.Model, "usage": usage,
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]string{"role": "assistant", "content": EchoPrefix + input},
			}},
		})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, text := range []string{EchoPrefix, input} {
		send(w, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}}}})
	}
	send(w, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
	send(w, map[string]any{"choices": []any{}, "usage": usage})
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func (s *OpenAIServer) count(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	if model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"calls": s.Calls(model)})
}

func (s *OpenAIServer) record(model string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.calls[model]; !ok && len(s.calls) >= s.maxModels {
		return false
	}
	s.calls[model]++
	return true
}

func lastUser(req chatRequest) (string, bool) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content, true
		}
	}
	return "", false
}

func words(s string) int { return len(strings.Fields(s)) }

func send(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
}
