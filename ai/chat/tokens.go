package chat

import "github.com/kbukum/gokit/ai"

// CountTokensApprox estimates the token count for a slice of messages using the
// 4-chars≈1-token heuristic. Use when a provider's native tokenizer is unavailable.
func CountTokensApprox(messages []Message) int {
	total := 0
	for _, m := range messages {
		switch msg := m.(type) {
		case UserMessage:
			total += (len(ai.TextOf(msg.Content)) + 3) / 4
		case AssistantMessage:
			total += (len(msg.Text()) + 3) / 4
			for _, tc := range msg.ToolCalls {
				total += (len(tc.Name) + 3) / 4
			}
		case SystemMessage:
			total += (len(msg.Content) + 3) / 4
		case ToolResultMessage:
			total += (len(msg.Content) + 3) / 4
		}
		total += 4
	}
	return total
}
