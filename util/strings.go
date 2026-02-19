package util

// StringInSlice checks if a string exists in a slice.
//
// Deprecated: Use Contains[string] instead.
func StringInSlice(s string, list []string) bool {
	return Contains(list, s)
}

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
