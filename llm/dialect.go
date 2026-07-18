package llm

import (
	"fmt"

	"github.com/kbukum/gokit/llm/internal/streamwire"
	"github.com/kbukum/gokit/provider/namedregistry"
)

// StreamFormat indicates how a provider delivers streaming responses.
type StreamFormat int

const (
	// StreamNDJSON uses newline-delimited JSON (one JSON object per line). Used by: Ollama native API.
	StreamNDJSON StreamFormat = iota
	// StreamSSE uses Server-Sent Events format. Used by: OpenAI, Anthropic, Azure OpenAI,
	// most cloud providers.
	StreamSSE
)

// String returns the human-readable name of the stream format.
func (f StreamFormat) String() string {
	switch f {
	case StreamNDJSON:
		return "NDJSON"
	case StreamSSE:
		return "SSE"
	default:
		return fmt.Sprintf("StreamFormat(%d)", int(f))
	}
}

// Dialect maps universal LLM types to/from a specific provider's HTTP format.
//
// Each LLM provider (Ollama, OpenAI, Anthropic, etc.) has its own Dialect implementation that handles the provider-specific request/response structure.
//
// Dialect implementations live in driver packages outside this package (e.g. github.com/kbukum/gokit/llm/providers/openai).
// Driver packages expose a Register function that callers invoke against an explicit [DialectRegistry] at startup.
type Dialect interface {
	// Name returns the dialect identifier (e.g., "ollama", "openai").
	Name() string

	// ChatPath returns the API endpoint path for chat completion (e.g., "/api/chat").
	ChatPath() string

	// HealthPath returns the health-check endpoint path. Empty means no health endpoint.
	HealthPath() string

	// BuildRequest maps a universal CompletionRequest to the provider's JSON request body.
	BuildRequest(req CompletionRequest) (any, error)

	// ParseResponse maps the provider's JSON response body to a universal CompletionResponse.
	ParseResponse(body []byte) (*CompletionResponse, error)

	// StreamFormat returns how this provider delivers streaming data.
	StreamFormat() StreamFormat

	// ParseStreamChunk extracts content (text and/or tool calls) from a single stream data chunk.
	// The returned chunk is llm-internal;
	// callers consume the assembled public StreamEvent values produced by Adapter.Stream/Provider.Stream.
	ParseStreamChunk(data []byte) (streamwire.Chunk, error)
}

// DialectRegistry stores LLM dialects by name.
//
// Registries are explicit, isolated, and thread-safe.
// Driver packages (for example github.com/kbukum/gokit/llm/providers/openai) expose Register(*DialectRegistry) functions that populate a registry during application startup.
// Pass the populated registry to [NewWithRegistry] to create an [Adapter] that resolves its dialect from the registry.
type DialectRegistry struct {
	inner *namedregistry.Registry[Dialect]
}

// NewDialectRegistry creates an isolated dialect registry.
func NewDialectRegistry() *DialectRegistry {
	return &DialectRegistry{inner: namedregistry.New[Dialect]("llm")}
}

// Register adds a dialect to the registry.
// It returns an error on programmer errors (empty name, nil dialect, duplicate name).
func (r *DialectRegistry) Register(name string, d Dialect) error {
	return r.inner.Register(name, d)
}

// Get retrieves a dialect by name.
func (r *DialectRegistry) Get(name string) (Dialect, error) {
	d, ok := r.inner.Get(name)
	if !ok {
		return nil, fmt.Errorf("llm: unknown dialect %q (forgot to register?)", name)
	}
	return d, nil
}

// Names returns the names of all registered dialects in deterministic order.
func (r *DialectRegistry) Names() []string {
	return r.inner.Names()
}
