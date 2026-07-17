package stage

import (
	"context"

	"github.com/kbukum/gokit/stream"
)

// Source produces a bounded stream of values of type T. Implementations own
// their cancellation: the returned pipeline must stop when its context is
// canceled.
type Source[T any] interface {
	// Name returns a stable identifier used for manifest keys and progress.
	Name() string
	// Stream returns a lazy pipeline of the source's values.
	Stream(ctx context.Context) *stream.Pipeline[T]
}

// Keyed is an optional capability a [Source] may implement to contribute a
// stable cache fingerprint distinct from its name.
type Keyed interface {
	// CacheKey returns a stable fingerprint of the source's configuration.
	CacheKey() string
}

// Bounded is an optional capability a [Source] may implement to advertise an
// upper bound on the number of items it will emit.
type Bounded interface {
	// MaxItems returns the item ceiling and whether one is known.
	MaxItems() (int, bool)
}

// sliceSource is an in-memory [Source] over a fixed slice, used for composition
// and tests.
type sliceSource[T any] struct {
	name  string
	items []T
}

// NewSliceSource returns a Source that emits items in order.
func NewSliceSource[T any](name string, items []T) Source[T] {
	return &sliceSource[T]{name: name, items: items}
}

func (s *sliceSource[T]) Name() string { return s.name }

func (s *sliceSource[T]) Stream(context.Context) *stream.Pipeline[T] {
	return stream.FromSlice(s.items)
}

func (s *sliceSource[T]) MaxItems() (int, bool) { return len(s.items), true }

// CacheKey returns a source's fingerprint: its [Keyed] CacheKey when
// implemented, otherwise its name.
func CacheKey[T any](s Source[T]) string {
	if k, ok := s.(Keyed); ok {
		return k.CacheKey()
	}
	return s.Name()
}

// MaxItems returns a source's item ceiling when it implements [Bounded].
func MaxItems[T any](s Source[T]) (int, bool) {
	if b, ok := s.(Bounded); ok {
		return b.MaxItems()
	}
	return 0, false
}
