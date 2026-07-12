package common

// EstimateTokens provides a rough token count estimate using the
// ~4 characters per token heuristic. This is useful for pre-flight
// checks before sending requests. For accurate counts, use the
// provider's native tokenizer.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4
}
