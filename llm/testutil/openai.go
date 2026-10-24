package testutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
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
	// VisionModelPrefix names models that accept images. Other models refuse an image with 400.
	VisionModelPrefix = "vision-"
	// EchoModel and VisionModel are the models GET /v1/models lists by default.
	EchoModel   = "echo"
	VisionModel = VisionModelPrefix + "echo"
	// maxImageSide bounds a decoded image's width and height.
	maxImageSide = 1024
)

// OpenAIServer is a deterministic OpenAI-compatible chat model for tests. It answers POST /v1/chat/completions with "echo: " followed by the last user message's text and reports word-count usage, streamed as server-sent events when the request asks for a stream. A model named with [VisionModelPrefix] answers a message carrying a base64 PNG data URL with the image's dominant color, "red", "green", "blue", "yellow", "black" or "white", or "unknown"; other models refuse images. GET /v1/models lists [EchoModel] and [VisionModel], or the names given to [WithModels]; any name is still accepted on chat. It counts calls per model, served over GET /calls?model=<name> and [OpenAIServer.Calls]. Models named with [FailModelPrefix] are counted and then fail with 502. It requires a bearer token unless built [WithKeyless], and never reads or stores its value. It is safe for concurrent use; serve it with httptest or an http.Server.
type OpenAIServer struct {
	mux       *http.ServeMux
	maxModels int
	listed    []string
	keyless   bool

	mu    sync.Mutex
	calls map[string]int
}

// OpenAIServerOption configures an [OpenAIServer].
type OpenAIServerOption func(*OpenAIServer)

// WithMaxModels bounds how many distinct model names are counted. Calls to a new model past the bound answer 503. A non-positive value keeps [DefaultMaxModels].
func WithMaxModels(n int) OpenAIServerOption {
	return func(s *OpenAIServer) {
		if n > 0 {
			s.maxModels = n
		}
	}
}

// WithModels sets the names GET /v1/models lists. An empty list keeps the defaults.
func WithModels(names ...string) OpenAIServerOption {
	return func(s *OpenAIServer) {
		if len(names) > 0 {
			s.listed = append([]string(nil), names...)
		}
	}
}

// WithKeyless answers chat without a bearer token, like a local runtime such as Ollama.
func WithKeyless() OpenAIServerOption {
	return func(s *OpenAIServer) { s.keyless = true }
}

// NewOpenAIServer returns a fake chat model with no recorded calls.
func NewOpenAIServer(opts ...OpenAIServerOption) *OpenAIServer {
	s := &OpenAIServer{maxModels: DefaultMaxModels, calls: map[string]int{}, listed: []string{EchoModel, VisionModel}}
	for _, opt := range opts {
		opt(s)
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /v1/chat/completions", s.chat)
	s.mux.HandleFunc("GET /v1/models", s.models)
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
		Role    string  `json:"role"`
		Content content `json:"content"`
	} `json:"messages"`
}

// content is a message's text and image data URLs. OpenAI sends it as a string or as an array of typed parts.
type content struct {
	text   string
	images []string
}

func (c *content) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &c.text); err == nil {
		return nil
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(b, &parts); err != nil {
		return err
	}
	var texts []string
	for _, p := range parts {
		switch p.Type {
		case "text":
			texts = append(texts, p.Text)
		case "image_url":
			c.images = append(c.images, p.ImageURL.URL)
		default:
			return fmt.Errorf("unsupported content part %q", p.Type)
		}
	}
	c.text = strings.Join(texts, " ")
	return nil
}

func (s *OpenAIServer) chat(w http.ResponseWriter, r *http.Request) {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); !s.keyless && (!ok || token == "") {
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
	message, ok := lastUser(req)
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
	text, status, refusal := reply(req.Model, message)
	if status != 0 {
		http.Error(w, refusal, status)
		return
	}

	prompt, completion := words(message.text), words(text)
	usage := map[string]int{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": prompt + completion}
	w.Header().Set("Cache-Control", "no-store")
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "chat.completion", "model": req.Model, "usage": usage,
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]string{"role": "assistant", "content": text},
			}},
		})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, part := range chunks(text) {
		send(w, map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": part}}}})
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

func (s *OpenAIServer) models(w http.ResponseWriter, _ *http.Request) {
	data := make([]map[string]any, 0, len(s.listed))
	for _, name := range s.listed {
		data = append(data, map[string]any{"id": name, "object": "model", "created": 0, "owned_by": "gokit-testutil"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
}

func lastUser(req chatRequest) (content, bool) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content, true
		}
	}
	return content{}, false
}

// reply returns the answer text, or a refusal status and message.
func reply(model string, msg content) (answer string, status int, reason string) {
	if len(msg.images) == 0 {
		return EchoPrefix + msg.text, 0, ""
	}
	if !strings.HasPrefix(model, VisionModelPrefix) {
		return "", http.StatusBadRequest, "model does not accept images"
	}
	color, err := dominantColor(msg.images[len(msg.images)-1])
	if err != nil {
		return "", http.StatusBadRequest, "invalid image"
	}
	return color, 0, ""
}

// dominantColor decodes a base64 PNG data URL and names the color of its center pixel.
func dominantColor(url string) (string, error) {
	encoded, ok := strings.CutPrefix(url, "data:image/png;base64,")
	if !ok {
		return "", errors.New("only base64 PNG data URLs are accepted")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return "", errors.New("image size out of bounds")
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	return colorName(img), nil
}

func colorName(img image.Image) string {
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	const strong, weak = 0xc000, 0x4000
	level := func(v uint32) int {
		switch {
		case v >= strong:
			return 1
		case v <= weak:
			return 0
		default:
			return -1
		}
	}
	switch [3]int{level(r), level(g), level(bl)} {
	case [3]int{1, 0, 0}:
		return "red"
	case [3]int{0, 1, 0}:
		return "green"
	case [3]int{0, 0, 1}:
		return "blue"
	case [3]int{1, 1, 0}:
		return "yellow"
	case [3]int{0, 0, 0}:
		return "black"
	case [3]int{1, 1, 1}:
		return "white"
	default:
		return "unknown"
	}
}

// chunks splits an echo answer into its prefix and the rest, so streams send more than one delta.
func chunks(text string) []string {
	if rest, ok := strings.CutPrefix(text, EchoPrefix); ok {
		return []string{EchoPrefix, rest}
	}
	return []string{text}
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
