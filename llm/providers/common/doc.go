// Package common holds error and rate-limit parsing shared by LLM providers.
//
// Helpers such as [ParseOpenAIError], [ParseAnthropicError],
// [ParseGeminiError], and [ParseRateLimitHeaders] translate vendor-specific
// error bodies and headers into a uniform [APIError] and [RateLimitInfo] so
// each provider does not re-implement the mapping.
package common
