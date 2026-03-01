package llm

import (
	"fmt"
	"sync"
)

// StreamFormat indicates how a provider delivers streaming responses.
type StreamFormat int

const (
	// StreamNDJSON uses newline-delimited JSON (one JSON object per line).
	// Used by: Ollama native API.
	StreamNDJSON StreamFormat = iota
	// StreamSSE uses Server-Sent Events format.
	// Used by: OpenAI, Anthropic, Azure OpenAI, most cloud providers.
	StreamSSE
)

// Dialect maps universal LLM types to/from a specific provider's HTTP format.
//
// Each LLM provider (Ollama, OpenAI, Anthropic, etc.) has its own Dialect
// implementation that handles the provider-specific request/response structure.
//
// Dialect implementations should live OUTSIDE this package:
//   - In separate gokit sub-modules (e.g., gokit/llm-ollama, gokit/llm-openai)
//   - In project code (e.g., internal/llm/dialects/)
//   - In community packages
//
// Register dialects at startup using [RegisterDialect], or pass directly to [NewWithDialect].
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

	// ParseStreamChunk extracts content from a single stream data chunk.
	// Returns the text content and whether the stream is complete.
	ParseStreamChunk(data []byte) (content string, done bool, err error)
}

// --- Dialect Registry ---

var (
	dialectsMu sync.RWMutex
	dialects   = map[string]Dialect{}
)

// RegisterDialect adds a dialect to the global registry.
// Typically called from init() in dialect driver packages:
//
//	func init() {
//	    llm.RegisterDialect("ollama", &Dialect{})
//	}
//
// Importing the driver package registers the dialect as a side-effect:
//
//	import _ "github.com/your-org/llm-ollama"
func RegisterDialect(name string, d Dialect) {
	dialectsMu.Lock()
	defer dialectsMu.Unlock()
	dialects[name] = d
}

// GetDialect retrieves a dialect by name from the global registry.
func GetDialect(name string) (Dialect, error) {
	dialectsMu.RLock()
	defer dialectsMu.RUnlock()
	d, ok := dialects[name]
	if !ok {
		return nil, fmt.Errorf("llm: unknown dialect %q (forgot to import driver?)", name)
	}
	return d, nil
}

// Dialects returns the names of all registered dialects.
func Dialects() []string {
	dialectsMu.RLock()
	defer dialectsMu.RUnlock()
	names := make([]string, 0, len(dialects))
	for name := range dialects {
		names = append(names, name)
	}
	return names
}
