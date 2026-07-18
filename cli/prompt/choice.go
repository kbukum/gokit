package prompt

// ChoiceID is the stable identifier a caller uses to recognize a chosen [Choice].
//
// It is opaque data (not a closure):
// the caller maps it back to domain meaning after the prompt returns.
type ChoiceID string

// Choice is a single selectable option: pure data carrying an id, a human label,
// an optional annotation line, and whether it is the recommended default.
type Choice struct {
	id          ChoiceID
	label       string
	annotation  string
	recommended bool
}

// NewChoice creates a choice from an id and human-readable label.
func NewChoice(id ChoiceID, label string) Choice {
	return Choice{id: id, label: label}
}

// WithAnnotation attaches a secondary annotation line (for example "detected in
// go.mod") and returns the choice.
func (c Choice) WithAnnotation(annotation string) Choice {
	c.annotation = annotation
	return c
}

// Recommended marks this choice as the recommended default and returns it.
//
// In [ModeNonInteractive] a select resolves to the recommended choice,
// and an interactive prompt offers it when the answer is left blank.
func (c Choice) Recommended() Choice {
	c.recommended = true
	return c
}

// ID returns the stable identifier of this choice.
func (c Choice) ID() ChoiceID { return c.id }

// Label returns the human-readable label.
func (c Choice) Label() string { return c.label }

// Annotation returns the optional annotation line ("" when unset).
func (c Choice) Annotation() string { return c.annotation }

// IsRecommended reports whether this choice is the recommended default.
func (c Choice) IsRecommended() bool { return c.recommended }
