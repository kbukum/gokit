// Package common holds error, rate-limit, and request-extension helpers shared
// by LLM providers.
//
// Helpers such as [ParseOpenAIError], [ParseAnthropicError],
// [ParseGeminiError], and [ParseRateLimitHeaders] translate vendor-specific
// error bodies and headers into a uniform [APIError] and [RateLimitInfo] so
// each provider does not re-implement the mapping. [MergeExtra] folds a
// caller-supplied raw JSON object of provider-specific request extensions into
// an outgoing request body, failing closed on malformed input.
package common
