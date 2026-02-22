package provider

import "context"

// RequestResponse represents a provider that takes one input and returns one output.
// This covers: HTTP calls, gRPC unary, subprocess exec, SQL queries, S3 operations.
type RequestResponse[I, O any] interface {
	Provider
	Execute(ctx context.Context, input I) (O, error)
}

// Stream represents a provider that takes one input and returns multiple outputs.
// This covers: gRPC server-stream, subprocess stdout pipe, SSE, chunked HTTP.
type Stream[I, O any] interface {
	Provider
	Execute(ctx context.Context, input I) (Iterator[O], error)
}

// Sink represents a provider that accepts input with no meaningful output.
// This covers: Kafka produce, webhook, push notification, logging.
type Sink[I any] interface {
	Provider
	Send(ctx context.Context, input I) error
}

// Duplex represents a provider with bidirectional communication.
// This covers: WebSocket, gRPC bidi-stream, long-running subprocess.
type Duplex[I, O any] interface {
	Provider
	Open(ctx context.Context) (DuplexStream[I, O], error)
}
