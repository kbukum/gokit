package payload

// DefaultMaxInMemoryBytes is the default cap on a single in-memory payload
// or bounded file read (8 MiB), mirroring rskit's dataset limits.
const DefaultMaxInMemoryBytes int64 = 8 * 1024 * 1024

// DefaultStreamBuffer is the default bound on a stage's in-flight buffer.
const DefaultStreamBuffer = 64

// Limits bounds the resources a dataset stage may consume.
// A payload larger than MaxInMemoryBytes must spill to a file rather than be held in memory,
// and StreamBuffer caps a stage's in-flight buffer to apply backpressure.
type Limits struct {
	// MaxInMemoryBytes caps a single in-memory payload or bounded file read.
	MaxInMemoryBytes int64
	// StreamBuffer caps the number of in-flight records a stage buffers.
	StreamBuffer int
}

// DefaultLimits returns the default bounds: 8 MiB in-memory cap and a 64-record stream buffer.
func DefaultLimits() Limits {
	return Limits{
		MaxInMemoryBytes: DefaultMaxInMemoryBytes,
		StreamBuffer:     DefaultStreamBuffer,
	}
}

// WithDefaults fills any zero field with its default
// so callers may pass a partially-populated Limits.
func (l Limits) WithDefaults() Limits {
	if l.MaxInMemoryBytes <= 0 {
		l.MaxInMemoryBytes = DefaultMaxInMemoryBytes
	}
	if l.StreamBuffer <= 0 {
		l.StreamBuffer = DefaultStreamBuffer
	}
	return l
}
