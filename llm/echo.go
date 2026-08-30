package llm

import (
	"context"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

// EchoName is the provider name advertised by [Echo].
const EchoName = "echo"

// Echo is a deterministic, dependency-free [Provider] that replies with the text of the most recent user message.
//
// It is a real provider for local composition and downstream tests — the LLM counterpart of the inference echo adapter and the in-memory embedding provider — so consumers never hand-roll a bespoke double. Reported [Usage] counts input tokens over the full prompt (system prompt plus messages) and output tokens over the echoed reply using the shared [chat.CountTokensApprox] approximation, so results are stable across runs. The zero value is ready to use.
type Echo struct{}

// Name reports the provider name [EchoName].
func (Echo) Name() string { return EchoName }

// IsAvailable always reports true: Echo needs no external dependency.
func (Echo) IsAvailable(context.Context) bool { return true }

// Capabilities reports streaming support and nothing else — Echo has no tool use, vision, or JSON mode.
func (Echo) Capabilities() Capabilities {
	return Capabilities{Streaming: true}
}

// CountTokens counts tokens over messages using the shared approximation, the same rule Execute uses for usage accounting.
func (Echo) CountTokens(messages []chat.Message) int {
	return chat.CountTokensApprox(messages)
}

// Execute returns the concatenated text of the most recent user message (empty when the request carries no user text), preserving the requested model name and defaulting to [EchoName] when the request omits it.
func (Echo) Execute(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return CompletionResponse{}, err
	}
	return echoResponse(req), nil
}

// Stream emits a MessageStart, a TextDelta carrying the echoed reply (omitted when the reply is empty), a UsageDelta, and a terminal [MessageComplete]. Every send is guarded by ctx, so a canceled context stops the stream without leaking the producer goroutine.
func (e Echo) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp := echoResponse(req)

	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)
		send := func(ev StreamEvent) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(MessageStart{Role: chat.RoleAssistant, Model: resp.Model}) {
			return
		}
		if text := resp.Text(); text != "" {
			if !send(TextDelta{Text: text}) {
				return
			}
		}
		if !send(UsageDelta{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}) {
			return
		}
		send(MessageComplete{Response: resp})
	}()
	return ch, nil
}

// echoResponse builds the deterministic completion for req.
func echoResponse(req CompletionRequest) CompletionResponse {
	reply := latestUserText(req.Messages)
	outputTokens := 0
	if reply != "" {
		outputTokens = chat.CountTokensApprox([]chat.Message{chat.User(reply)})
	}

	model := req.Model
	if model == "" {
		model = EchoName
	}

	return CompletionResponse{
		Message: chat.Assistant(reply),
		Model:   model,
		Usage: Usage{
			InputTokens:  inputTokens(req),
			OutputTokens: outputTokens,
		},
		StopReason: chat.FinishReasonStop,
	}
}

// inputTokens approximates the prompt token count over the full request — the system prompt (when set) plus all messages — so a request carrying a separate [CompletionRequest.SystemPrompt] is not underreported.
func inputTokens(req CompletionRequest) int {
	messages := req.Messages
	if req.SystemPrompt != "" {
		messages = append([]chat.Message{chat.System(req.SystemPrompt)}, req.Messages...)
	}
	return chat.CountTokensApprox(messages)
}

// latestUserText returns the text of the most recent user message, or empty when the conversation carries no user turn.
func latestUserText(messages []chat.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if user, ok := messages[i].(chat.UserMessage); ok {
			return ai.TextOf(user.Content)
		}
	}
	return ""
}
