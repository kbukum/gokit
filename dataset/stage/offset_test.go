package stage

import "testing"

type offsetItem struct{ off int }

func (o offsetItem) SourceOffset() (int, bool) { return o.off, true }

func TestOffsetOf(t *testing.T) {
	t.Parallel()
	if off, ok := OffsetOf(offsetItem{off: 7}); !ok || off != 7 {
		t.Errorf("OffsetOf(offsetItem{7}) = %d, %v; want 7, true", off, ok)
	}
	if off, ok := OffsetOf("plain"); ok || off != 0 {
		t.Errorf("OffsetOf(non-Offsetted) = %d, %v; want 0, false", off, ok)
	}
}
