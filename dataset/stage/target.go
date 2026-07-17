package stage

import (
	"context"

	"github.com/kbukum/gokit/stream"
)

// PublishResult summarizes what a [Target] published.
type PublishResult struct {
	// TargetName is the publishing target's name.
	TargetName string `json:"target_name"`
	// Location identifies where records were published (path, URI, ...).
	Location string `json:"location"`
	// RecordsPublished counts the records the target accepted.
	RecordsPublished int `json:"records_published"`
	// Message is an optional human-readable note.
	Message string `json:"message,omitempty"`
}

// Target consumes a stream of values of type T and publishes them, returning a
// [PublishResult]. Publishing must honor context cancellation.
type Target[T any] interface {
	// Name returns a stable identifier for diagnostics and results.
	Name() string
	// Publish drains items and returns what was published.
	Publish(ctx context.Context, items *stream.Pipeline[T]) (PublishResult, error)
}

// SliceTarget collects published values into a slice, for composition and
// tests.
type SliceTarget[T any] struct {
	name    string
	records []T
}

// NewSliceTarget returns a Target that accumulates published values in memory.
func NewSliceTarget[T any](name string) *SliceTarget[T] {
	return &SliceTarget[T]{name: name}
}

// Name returns the target's identifier.
func (t *SliceTarget[T]) Name() string { return t.name }

// Records returns the values published so far.
func (t *SliceTarget[T]) Records() []T { return t.records }

// Publish drains items into the target's in-memory slice.
func (t *SliceTarget[T]) Publish(ctx context.Context, items *stream.Pipeline[T]) (PublishResult, error) {
	collected, err := stream.Collect(ctx, items)
	if err != nil {
		return PublishResult{}, err
	}
	t.records = append(t.records, collected...)
	return PublishResult{
		TargetName:       t.name,
		Location:         "memory",
		RecordsPublished: len(collected),
	}, nil
}
