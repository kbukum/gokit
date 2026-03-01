package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/provider"
)

// Complete is a convenience helper: sends system + user prompts and returns the text response.
// Accepts gokit RequestResponse so it works with any wrapped/composed provider
// (e.g., WithResilience, middleware chains).
func Complete(ctx context.Context, p provider.RequestResponse[CompletionRequest, CompletionResponse], system, user string) (string, error) {
	resp, err := p.Execute(ctx, CompletionRequest{
		SystemPrompt: system,
		Messages:     []Message{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// CompleteStructured sends a prompt expecting JSON and unmarshals the response into result.
// Accepts gokit RequestResponse so it works with any wrapped/composed provider.
// It appends JSON formatting instructions to the system prompt.
func CompleteStructured(ctx context.Context, p provider.RequestResponse[CompletionRequest, CompletionResponse], system, user string, result any) error {
	system += "\n\nIMPORTANT: Respond with ONLY the JSON object. " +
		"No markdown, no code blocks, no explanations. " +
		"Start with { and end with }."

	resp, err := p.Execute(ctx, CompletionRequest{
		SystemPrompt: system,
		Messages:     []Message{{Role: "user", Content: user}},
	})
	if err != nil {
		return err
	}

	content := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(content), result); err != nil {
		return fmt.Errorf("llm: unmarshal structured response: %w", err)
	}
	return nil
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
