// Package dialect holds request-assembly helpers shared by LLM provider dialects. [MergeExtra] folds a caller-supplied raw JSON object of provider-specific request extensions into an outgoing request body, failing closed on malformed input.
//
// It is internal to the providers module: the map[string]any it operates on is an implementation detail of dialect request building and must not leak into any public provider API.
package dialect
