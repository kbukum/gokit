package util

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}

// Deref returns the value pointed to by p, or the zero value if p is nil.
func Deref[T any](p *T) T {
	if p != nil {
		return *p
	}
	var zero T
	return zero
}
