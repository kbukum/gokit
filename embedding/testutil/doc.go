// Package testutil provides deterministic [embedding.Provider] test doubles.
//
// [FakeProvider] mirrors llm/testutil.FakeProvider for the embedding surface: it embeds text into fixed-dimension vectors deterministically and supports injected errors, a dynamic responder, pinned per-text vectors, and a block-until-cancel mode, so embedding-backed code (semantic metrics, retrieval, reranking) can be exercised — including failure, dimension-mismatch, and cancellation paths — without a network model or external service.
package testutil
