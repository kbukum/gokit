package stage

// Validator is the pluggable per-item validation seam the collector applies before an item is published. It is generic over the item type so validation is not pinned to any concrete family; a nil Validator accepts every item.
type Validator[T any] interface {
	// Validate returns a typed error when v is rejected, or nil when accepted.
	Validate(v T) error
}

// ValidatorFunc adapts a function into a [Validator].
type ValidatorFunc[T any] func(T) error

// Validate invokes the wrapped function.
func (f ValidatorFunc[T]) Validate(v T) error { return f(v) }
