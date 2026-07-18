package stage

// Label classifies an item as real (collected from a genuine source) or AI (synthetic/augmented),
// so a run can aggregate real/ai/total stats.
type Label int

const (
	// LabelReal marks a genuine, non-synthetic item. It is the zero value
	// so an item that does not classify itself is treated as real.
	LabelReal Label = iota
	// LabelAI marks a synthetic or augmented item.
	LabelAI
)

// String returns the lowercase label name ("real" or "ai").
func (l Label) String() string {
	switch l {
	case LabelAI:
		return "ai"
	default:
		return "real"
	}
}

// Labeled is an optional capability an item may implement to classify itself as real or AI.
// Items that do not implement it are treated as [LabelReal].
type Labeled interface {
	// Label reports whether the item is real or AI.
	Label() Label
}

// LabelOf returns an item's [Label] when it implements [Labeled], and [LabelReal] otherwise.
func LabelOf[T any](v T) Label {
	if l, ok := any(v).(Labeled); ok {
		return l.Label()
	}
	return LabelReal
}
