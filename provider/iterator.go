package provider

import "context"

// Iterator provides pull-based sequential access to a stream of values.
// The consumer calls Next() to retrieve values one at a time.
// Close must be called when done to release resources.
type Iterator[T any] interface {
	// Next returns the next value. Returns (zero, false, nil) when exhausted.
	Next(ctx context.Context) (T, bool, error)
	// Close releases any resources held by the iterator.
	Close() error
}

// DuplexStream provides bidirectional communication over a single connection.
type DuplexStream[I, O any] interface {
	// Send writes a value to the remote end.
	Send(I) error
	// Recv reads a value from the remote end. Returns io.EOF when closed.
	Recv() (O, error)
	// Close terminates the stream.
	Close() error
}
