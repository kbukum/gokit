package sample

import (
	"testing"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
)

func TestItemCapabilities(t *testing.T) {
	t.Parallel()
	p, err := payload.FromBytes([]byte("x"), payload.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	it := New("a.bin", stage.LabelAI, 4, p)
	if it.Name() != "a.bin" {
		t.Fatalf("Name = %q", it.Name())
	}
	if it.Label() != stage.LabelAI {
		t.Fatalf("Label = %v; want AI", it.Label())
	}
	if off, ok := it.SourceOffset(); !ok || off != 4 {
		t.Fatalf("SourceOffset = %d, %v; want 4, true", off, ok)
	}
	if stage.LabelOf(it) != stage.LabelAI {
		t.Fatalf("LabelOf = %v; want AI", stage.LabelOf(it))
	}
	if off, ok := stage.OffsetOf(it); !ok || off != 4 {
		t.Fatalf("OffsetOf = %d, %v; want 4, true", off, ok)
	}
}
