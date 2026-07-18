package llm

import (
	"context"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/provider"
)

// Provider is the canonical LLM-provider contract.
//
// Provider natively embeds [provider.RequestResponse]
// so any Provider drops into dag / pipeline / chain / worker consumers without a bridge.
//
// Streaming remains a first-class extension via the named Stream method.
// We do not embed [provider.Stream] because its Execute method conflicts with [provider.RequestResponse.Execute].
//
// Required methods (by transitive embedding):
//   - Name() string                                                   // provider.Provider
//   - IsAvailable(ctx context.Context) bool                           // provider.Provider
//   - Execute(ctx, CompletionRequest) (CompletionResponse, error)     // RequestResponse
//   - Stream(ctx, CompletionRequest) (<-chan StreamEvent, error)
//   - Capabilities() Capabilities
//   - CountTokens(messages []chat.Message) int
type Provider interface {
	provider.RequestResponse[CompletionRequest, CompletionResponse]
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error)
	Capabilities() Capabilities
	CountTokens(messages []chat.Message) int
}

type Capabilities = ai.Capabilities
