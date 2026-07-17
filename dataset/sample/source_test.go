package sample

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
	"github.com/kbukum/gokit/stream"
)

func TestDirSourceProducesLabeledItems(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"b.bin", "a.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	src := NewDirSource("s", dir, stage.LabelReal, payload.DefaultLimits())
	items, err := stream.Collect(context.Background(), src.Stream(context.Background()))
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items; want 2", len(items))
	}
	// Sorted order: a.bin at offset 0, b.bin at offset 1.
	if items[0].Name() != "a.bin" || items[0].offset != 0 {
		t.Fatalf("item[0] = %s@%d; want a.bin@0", items[0].Name(), items[0].offset)
	}
	if items[1].Name() != "b.bin" || items[1].offset != 1 {
		t.Fatalf("item[1] = %s@%d; want b.bin@1", items[1].Name(), items[1].offset)
	}
}

func TestDirSourceMissingDirFailsClosed(t *testing.T) {
	t.Parallel()
	src := NewDirSource("s", filepath.Join(t.TempDir(), "absent"), stage.LabelReal, payload.DefaultLimits())
	if _, err := stream.Collect(context.Background(), src.Stream(context.Background())); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestDirSourceOversizedFileFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := NewDirSource("s", dir, stage.LabelReal, payload.Limits{MaxInMemoryBytes: 4})
	if _, err := stream.Collect(context.Background(), src.Stream(context.Background())); err == nil {
		t.Fatal("expected oversized file to be rejected")
	}
}

func TestSliceSource(t *testing.T) {
	t.Parallel()
	p, _ := payload.FromBytes([]byte("x"), payload.DefaultLimits())
	src := NewSliceSource("s", []Item{New("a", stage.LabelReal, 0, p)})
	items, err := stream.Collect(context.Background(), src.Stream(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items; want 1", len(items))
	}
}
