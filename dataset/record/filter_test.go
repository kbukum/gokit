package record

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/stream"
)

func TestFilter(t *testing.T) {
	t.Parallel()
	p := recordPipeline(
		map[string]Value{"keep": true},
		map[string]Value{"keep": false},
		map[string]Value{"keep": true},
	)
	filtered := Filter(p, func(r Record) bool {
		v, _ := r.Get("keep")
		return v == true
	})
	records, err := stream.Collect(context.Background(), filtered)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("filtered to %d; want 2", len(records))
	}
}

func TestSelectColumns(t *testing.T) {
	t.Parallel()
	p := recordPipeline(map[string]Value{"a": "1", "b": "2", "c": "3"})
	selected := SelectColumns(p, []string{"a", "c"})
	records, err := stream.Collect(context.Background(), selected)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if records[0].Len() != 2 {
		t.Fatalf("selected record has %d fields; want 2", records[0].Len())
	}
	if _, ok := records[0].Get("b"); ok {
		t.Fatal("column b should be dropped")
	}
}
