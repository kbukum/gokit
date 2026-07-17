package stage

import "testing"

type aiItem struct{}

func (aiItem) Label() Label { return LabelAI }

func TestLabelString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		label Label
		want  string
	}{
		{LabelReal, "real"},
		{LabelAI, "ai"},
		{Label(99), "real"},
	}
	for _, tt := range tests {
		if got := tt.label.String(); got != tt.want {
			t.Errorf("Label(%d).String() = %q; want %q", tt.label, got, tt.want)
		}
	}
}

func TestLabelOf(t *testing.T) {
	t.Parallel()
	if got := LabelOf(aiItem{}); got != LabelAI {
		t.Errorf("LabelOf(aiItem) = %v; want LabelAI", got)
	}
	if got := LabelOf("plain"); got != LabelReal {
		t.Errorf("LabelOf(non-Labeled) = %v; want LabelReal", got)
	}
}
