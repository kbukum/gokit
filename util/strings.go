package util

// Coalesce returns the first non-zero value, or the zero value if all are zero.
func Coalesce[T comparable](values ...T) T {
	var zero T
	for _, v := range values {
		if v != zero {
			return v
		}
	}
	return zero
}
