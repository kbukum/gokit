// Package streamwire assembles streamed LLM wire chunks into complete values.
//
// It merges incremental tool-call deltas ([MergeToolDelta]) and content
// [Chunk]s arriving over a streaming response into coherent [ToolCall] and
// tool-use blocks, rejecting malformed JSON so callers receive validated,
// fully-formed results.
package streamwire
