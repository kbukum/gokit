package tiktoken

import (
	"testing"

	"github.com/kbukum/gokit/llm"
)

// mustCount counts text, failing the test on a counter error.
func mustCount(t *testing.T, c llm.TokenCounter, text string) int {
	t.Helper()
	n, err := c.Count(text)
	if err != nil {
		t.Fatalf("Count(%q): %v", text, err)
	}
	return n
}

func TestNewRejectsUnknownEncoding(t *testing.T) {
	t.Parallel()

	if _, err := New("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown encoding, got nil")
	}
}

func TestCounterName(t *testing.T) {
	t.Parallel()

	c, err := New(Cl100kBase)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := c.Name(), "tiktoken:cl100k_base"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestCounterEmptyIsZero(t *testing.T) {
	t.Parallel()

	c, err := New(Cl100kBase)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := mustCount(t, c, ""); got != 0 {
		t.Errorf("Count(\"\") = %d, want 0", got)
	}
}

func TestCounterCountsAndIsDeterministic(t *testing.T) {
	t.Parallel()

	c, err := New(Cl100kBase)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const text = "the quick brown fox jumps over the lazy dog"
	first := mustCount(t, c, text)
	if first <= 0 {
		t.Fatalf("Count = %d, want positive", first)
	}
	if second := mustCount(t, c, text); first != second {
		t.Errorf("Count not deterministic: %d != %d", first, second)
	}
	// "tokens" encodes to a single BPE token under cl100k_base; a longer phrase
	// must not produce fewer tokens than a single word.
	if mustCount(t, c, "hello world this is a longer phrase") < mustCount(t, c, "hi") {
		t.Error("longer text produced fewer tokens than shorter text")
	}
}

func TestNewCounterFromConfig(t *testing.T) {
	t.Parallel()

	var counter llm.TokenCounter
	counter, err := NewCounter(Config{Encoding: string(O200kBase)})
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}
	if got, want := counter.Name(), "tiktoken:o200k_base"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if mustCount(t, counter, "hello") <= 0 {
		t.Error("Count(hello) = 0, want positive")
	}
}

// TestGoldenCountsPerEncoding pins exact counts for each supported encoding over
// ASCII, Unicode, and special-token-like text. Golden values (not just
// positive/monotonic assertions) catch a wrong encoding or a heuristic
// substitution: a regression that changed the encoding would change these counts.
func TestGoldenCountsPerEncoding(t *testing.T) {
	t.Parallel()

	const (
		ascii   = "The quick brown fox jumps over the lazy dog."
		unicode = "héllo wörld — café 日本語 🚀"
		special = "<|endoftext|> tokens and\tnewlines\n"
	)
	golden := map[Encoding]struct{ ascii, unicode, special int }{
		O200kBase:  {10, 11, 12},
		Cl100kBase: {10, 15, 12},
		P50kBase:   {10, 18, 13},
		R50kBase:   {10, 18, 13},
	}
	for enc, want := range golden {
		t.Run(string(enc), func(t *testing.T) {
			t.Parallel()
			c, err := New(enc)
			if err != nil {
				t.Fatalf("New(%s): %v", enc, err)
			}
			if got := mustCount(t, c, ascii); got != want.ascii {
				t.Errorf("Count(ascii) = %d, want %d", got, want.ascii)
			}
			if got := mustCount(t, c, unicode); got != want.unicode {
				t.Errorf("Count(unicode) = %d, want %d", got, want.unicode)
			}
			// Special-token-like text is counted as ordinary bytes (Count uses
			// EncodeOrdinary), never treated as control tokens.
			if got := mustCount(t, c, special); got != want.special {
				t.Errorf("Count(special) = %d, want %d", got, want.special)
			}
		})
	}
}
