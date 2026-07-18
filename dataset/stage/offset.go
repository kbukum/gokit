package stage

// Offsetted is an optional capability an item may implement to report the source offset it was fetched at, so a partial run can resume past it.
type Offsetted interface {
	// SourceOffset returns the item's offset within its source and whether one is known.
	SourceOffset() (int, bool)
}

// OffsetOf returns an item's source offset when it implements [Offsetted], and (0, false) otherwise.
func OffsetOf[T any](v T) (int, bool) {
	if o, ok := any(v).(Offsetted); ok {
		return o.SourceOffset()
	}
	return 0, false
}
