package llm

import "github.com/kbukum/gokit/ai/chat"

// TokenCounter is the canonical tokenization seam owned by llm: it counts the
// number of tokens a text string decomposes into.
//
// Implementations must be deterministic — the same input always yields the same
// count. Counting may fail: an exact tokenizer can surface an encode error at
// call time, so Count returns an error rather than substituting a success-shaped
// count that a caller could not distinguish from a real one. Core ships only the
// dependency-free [HeuristicTokenCounter]; exact tokenizers (OpenAI BPE,
// HuggingFace) live in nested contrib sub-modules under llm/tokenizer/ and are
// injected explicitly, never wired at import time.
type TokenCounter interface {
	// Name returns a stable identifier for the tokenization strategy — for
	// example "heuristic", "tiktoken:cl100k_base", or
	// "huggingface:<fingerprint>". Consumers that persist token counts (bench
	// metrics, run provenance) use it to avoid comparing counts produced by
	// incompatible strategies. It must be deterministic and stable across
	// processes for a given configuration.
	Name() string
	// Count returns the number of tokens in text, or an error if the tokenizer
	// fails to encode it. An empty string yields 0 with no error.
	Count(text string) (int, error)
}

// HeuristicTokenCounter is a dependency-free approximate [TokenCounter]. It uses
// the shared 4-chars≈1-token estimate ([chat.ApproxTokens]), the same rule as
// gokit's chat helpers, rather than a second divergent heuristic. This is an
// estimate, not an exact tokenizer: reach for a llm/tokenizer contrib adapter
// (OpenAI BPE, HuggingFace) when precise, model-specific counts matter.
type HeuristicTokenCounter struct{}

// Name returns the stable strategy identifier "heuristic".
func (HeuristicTokenCounter) Name() string { return "heuristic" }

// Count returns the approximate token count of text (0 for an empty string). The
// heuristic never fails, so the error is always nil.
func (HeuristicTokenCounter) Count(text string) (int, error) {
	return chat.ApproxTokens(text), nil
}
