// Package llmeval adapts LLM providers to typed benchmark observations, separating content timing from reasoning, tools, and metadata.
//
// An optional counter counts raw dataset input and final response text only when runtime usage is missing. Counted usage does not include chat-envelope overhead or text added by the request mapper; use runtime-reported usage for those costs. Unary calls do not invent TTFT.
package llmeval
