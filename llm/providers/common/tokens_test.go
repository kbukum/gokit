package common

import "testing"

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty string should be 0 tokens")
	}
	// "hello world" = 11 chars → ~3 tokens
	est := EstimateTokens("hello world")
	if est < 2 || est > 4 {
		t.Errorf("expected 2-4 tokens for 'hello world', got %d", est)
	}
}
