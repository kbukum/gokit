// Package explain provides structured explanation generation using LLMs.
//
// It takes a set of analysis signals (name, value, label) and produces
// human-readable explanations with reasoning steps via an LLM provider.
//
// This package builds on gokit/llm's CompleteStructured pattern —
// it renders signals into a prompt template, calls the LLM, and
// parses the structured JSON response into an Explanation.
//
// Usage:
//
//	signals := []explain.Signal{
//	    {Name: "frequency_score", Value: 0.92, Label: "Frequency analysis"},
//	    {Name: "metadata_score", Value: 0.15, Label: "Metadata check"},
//	}
//
//	exp, err := explain.Generate(ctx, llmProvider, explain.Request{
//	    Signals:   signals,
//	    MaxTokens: 512,
//	})
package explain
