package record

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/stream"
)

func TestFileSourceReadsFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "in.csv")
	if err := os.WriteFile(path, []byte("name\na\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := NewFileSource("s", path, FormatCSV, payload.DefaultLimits())
	if src.CacheKey() != "csv:"+path {
		t.Fatalf("CacheKey = %q", src.CacheKey())
	}
	recs, err := stream.Collect(context.Background(), src.Stream(context.Background()))
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records; want 2", len(recs))
	}
}

func TestFileSourceMissingFileFailsClosed(t *testing.T) {
	t.Parallel()
	src := NewFileSource("s", filepath.Join(t.TempDir(), "absent.csv"), FormatCSV, payload.DefaultLimits())
	if _, err := stream.Collect(context.Background(), src.Stream(context.Background())); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFileTargetAccumulatesAcrossPublishes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "acc.jsonl")
	target := NewFileTarget("t", path, FormatJSONLines)

	if _, err := target.Publish(context.Background(), stream.FromSlice([]Record{
		New(map[string]Value{"name": "a"}),
	})); err != nil {
		t.Fatalf("first Publish error: %v", err)
	}
	pub, err := target.Publish(context.Background(), stream.FromSlice([]Record{
		New(map[string]Value{"name": "b"}),
	}))
	if err != nil {
		t.Fatalf("second Publish error: %v", err)
	}
	if pub.RecordsPublished != 2 {
		t.Fatalf("RecordsPublished = %d; want 2 (accumulated)", pub.RecordsPublished)
	}
	recs, err := ReadJSONLines(path, payload.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	all, err := stream.Collect(context.Background(), recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("file holds %d records; want 2 (no clobber)", len(all))
	}
}

func TestFileTargetWritesAtomically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")
	target := NewFileTarget("t", path, FormatJSONLines)
	items := stream.FromSlice([]Record{
		New(map[string]Value{"name": "a"}),
		New(map[string]Value{"name": "b"}),
	})
	pub, err := target.Publish(context.Background(), items)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if pub.RecordsPublished != 2 || pub.Location != path {
		t.Fatalf("PublishResult = %+v", pub)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected written output")
	}
}
