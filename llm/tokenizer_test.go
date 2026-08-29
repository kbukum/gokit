package llm

import "testing"

// mustCount counts text with c, failing the test on an unexpected error.
func mustCount(t *testing.T, c TokenCounter, text string) int {
	t.Helper()
	n, err := c.Count(text)
	if err != nil {
		t.Fatalf("Count(%q): %v", text, err)
	}
	return n
}

func TestHeuristicTokenCounterEmptyIsZero(t *testing.T) {
	t.Parallel()

	if got := mustCount(t, HeuristicTokenCounter{}, ""); got != 0 {
		t.Errorf("Count(\"\") = %d, want 0", got)
	}
}

func TestHeuristicTokenCounterKnownValues(t *testing.T) {
	t.Parallel()

	// byte length / 4, rounded up — matches chat.ApproxTokens exactly.
	cases := map[string]int{
		"a":        1,
		"abcd":     1,
		"abcde":    2,
		"abcdefgh": 2,
	}
	var c HeuristicTokenCounter
	for text, want := range cases {
		if got := mustCount(t, c, text); got != want {
			t.Errorf("Count(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestHeuristicTokenCounterIsDeterministic(t *testing.T) {
	t.Parallel()

	var c HeuristicTokenCounter
	const text = "the quick brown fox"
	first := mustCount(t, c, text)
	if second := mustCount(t, c, text); first != second {
		t.Errorf("Count not deterministic: %d != %d", first, second)
	}
}

func TestHeuristicTokenCounterGrowsWithLength(t *testing.T) {
	t.Parallel()

	var c HeuristicTokenCounter
	prev := 0
	text := ""
	for range 64 {
		text += "x"
		got := mustCount(t, c, text)
		if got < prev {
			t.Fatalf("count decreased as text grew: %d < %d", got, prev)
		}
		prev = got
	}
}

func TestHeuristicTokenCounterName(t *testing.T) {
	t.Parallel()

	if got := (HeuristicTokenCounter{}).Name(); got != "heuristic" {
		t.Errorf("Name() = %q, want %q", got, "heuristic")
	}
}

// TokenCounter is satisfied by the heuristic default.
var _ TokenCounter = HeuristicTokenCounter{}
