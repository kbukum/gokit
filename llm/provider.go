package llm

import (
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/provider"
)

// Provider is the canonical LLM-provider contract.
//
// Per locked decision D7 (NATIVE EMBED), Provider natively embeds
// [provider.Streamable] — the canonical "RequestResponse + named Stream"
// composition shipped by package provider — so any Provider drops into
// dag / pipeline / chain / worker consumers without a bridge.
//
// Note on Go method-set rules: [provider.RequestResponse] and
// [provider.Stream] both declare a method named `Execute` with different
// return types, so a single interface cannot embed both. [provider.Streamable]
// is the canonical Go-idiomatic composition for this case (RR `Execute` plus
// a separately-named `Stream` method returning a chunk channel) and is what
// we embed.
//
// Required methods (by transitive embedding):
//   - Name() string                                                   // provider.Provider
//   - IsAvailable(ctx context.Context) bool                           // provider.Provider
//   - Execute(ctx, CompletionRequest) (CompletionResponse, error)     // RequestResponse
//   - Stream(ctx, CompletionRequest) (<-chan StreamEvent, error)      // Streamable
//   - Capabilities() Capabilities
//   - CountTokens(messages []chat.Message) int
type Provider interface {
	provider.Streamable[CompletionRequest, CompletionResponse, StreamEvent]
	Capabilities() Capabilities
	CountTokens(messages []chat.Message) int
}

type Capabilities = ai.Capabilities
