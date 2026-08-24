// Package testutil provides deterministic test doubles for the gokit/llm
// package so tests across the AI layer (llm, agent, mcp, embedding consumers)
// share one hardened fake provider instead of hand-rolling per-test mocks.
//
// [FakeProvider] implements [llm.Provider] and is configured with functional
// options: a fixed or queued set of responses, a dynamic responder, an injected
// error, capabilities, a token counter, and a block-until-cancel mode for
// exercising context cancellation. It is safe for concurrent use.
//
// Example:
//
//	p := testutil.NewFakeProvider(
//		testutil.WithResponses(testutil.TextResponse("hello")),
//	)
//	resp, err := p.Execute(ctx, llm.CompletionRequest{})
package testutil
