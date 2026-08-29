package chat

import "github.com/kbukum/gokit/ai"

// ApproxTokens estimates the token count of a single text string using the
// 4-chars≈1-token heuristic (byte length divided by four, rounded up). An empty
// string yields 0. This is the canonical per-string estimate reused by
// [CountTokensApprox] and by the llm tokenizer port's heuristic default, so all
// approximate token counting shares one rule rather than diverging.
func ApproxTokens(text string) int {
	return (len(text) + 3) / 4
}

// CountTokensApprox estimates the token count for a slice of messages using the 4-chars≈1-token heuristic.
// Use when a provider's native tokenizer is unavailable.
func CountTokensApprox(messages []Message) int {
	total := 0
	for _, m := range messages {
		switch msg := m.(type) {
		case UserMessage:
			total += ApproxTokens(ai.TextOf(msg.Content))
		case AssistantMessage:
			total += ApproxTokens(msg.Text())
			for _, tc := range msg.ToolCalls {
				total += ApproxTokens(tc.Name)
			}
		case SystemMessage:
			total += ApproxTokens(msg.Content)
		case ToolResultMessage:
			total += ApproxTokens(msg.Content)
		}
		total += 4
	}
	return total
}
