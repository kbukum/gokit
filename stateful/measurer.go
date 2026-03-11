package stateful

import "context"

// Measurer defines how to measure accumulated values. The measurement is used
// by size-based triggers and capacity checks (MinSize, MaxSize).
//
// Examples:
//   - CountMeasurer: measures by number of items
//   - ByteSizeMeasurer: measures by total bytes
//   - TokenMeasurer: measures by token count (for LLM context)
//   - CustomMeasurer: custom measurement logic
type Measurer[V any] interface {
	// Measure returns the size/measurement of the given values.
	// The unit depends on the measurer (count, bytes, tokens, etc.).
	Measure(ctx context.Context, values []V) int
}

// CountMeasurer returns a measurer that counts the number of items.
// This is the default measurer if none is provided.
func CountMeasurer[V any]() Measurer[V] {
	return countMeasurer[V]{}
}

type countMeasurer[V any] struct{}

func (countMeasurer[V]) Measure(_ context.Context, values []V) int {
	return len(values)
}

// ByteSizeMeasurer returns a measurer that sums the length of byte slices.
// Only works with []byte values.
func ByteSizeMeasurer() Measurer[[]byte] {
	return byteSizeMeasurer{}
}

type byteSizeMeasurer struct{}

func (byteSizeMeasurer) Measure(_ context.Context, values [][]byte) int {
	total := 0
	for _, b := range values {
		total += len(b)
	}
	return total
}

// CustomMeasurer creates a measurer with custom logic.
func CustomMeasurer[V any](fn func(context.Context, []V) int) Measurer[V] {
	return customMeasurer[V]{fn: fn}
}

type customMeasurer[V any] struct {
	fn func(context.Context, []V) int
}

func (m customMeasurer[V]) Measure(ctx context.Context, values []V) int {
	return m.fn(ctx, values)
}
