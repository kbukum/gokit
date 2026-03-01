package provider

import "context"

// Streamable represents a provider that supports both single-response and streaming modes.
// Use this for providers where the same input can produce either a complete response
// or a stream of chunks, typically controlled by a flag in the input.
//
// Common examples: LLM chat completion (stream: true/false), SSE event feeds.
//
// I is the input/request type.
// O is the single-response output type (from Execute).
// C is the streamed chunk type (from Stream).
type Streamable[I, O, C any] interface {
	RequestResponse[I, O]
	// Stream sends a request and returns a channel of streamed chunks.
	// The channel is closed when the stream ends or an error occurs.
	// Errors are delivered as chunk values (e.g., check a chunk's Err field).
	Stream(ctx context.Context, input I) (<-chan C, error)
}
