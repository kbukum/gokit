package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/provider"
)

// Complete is a convenience helper: sends system + user prompts and returns the text response.
func Complete(ctx context.Context, p provider.RequestResponse[CompletionRequest, CompletionResponse], system, user string) (string, error) {
	resp, err := p.Execute(ctx, CompletionRequest{
		SystemPrompt: system,
		Messages:     []chat.Message{chat.User(user)},
	})
	if err != nil {
		return "", err
	}
	return resp.Text(), nil
}

// CompleteStructured sends a prompt expecting JSON
// and decodes the response into a value of type T. The model output is untrusted:
// it is decoded into the concrete type T (a typed trust boundary that rejects shape-mismatched JSON) rather than an opaque map,
// and a decode failure returns the zero T with an error instead of a partially populated value.
func CompleteStructured[T any](ctx context.Context, p provider.RequestResponse[CompletionRequest, CompletionResponse], system, user string) (T, error) {
	var result T
	system += "\n\nIMPORTANT: Respond with ONLY the JSON object. " +
		"No markdown, no code blocks, no explanations. " +
		"Start with { and end with }."

	resp, err := p.Execute(ctx, CompletionRequest{
		SystemPrompt: system,
		Messages:     []chat.Message{chat.User(user)},
	})
	if err != nil {
		return result, err
	}

	content := extractJSON(resp.Text())
	var decoded T
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return result, fmt.Errorf("llm: unmarshal structured response: %w", err)
	}
	return decoded, nil
}

// extractJSON pulls a JSON object from LLM output that may contain markdown fences.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s[3:], "\n"); idx >= 0 {
			s = s[3+idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
