package record

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/dataset/payload"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/stream"
)

func TestParseCSV(t *testing.T) {
	t.Parallel()
	records, err := ParseCSV([]byte("name,age\nalice,30\nbob,25\n"))
	if err != nil {
		t.Fatalf("ParseCSV error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records; want 2", len(records))
	}
	if v, _ := records[0].Get("name"); v != "alice" {
		t.Errorf("record0 name = %v; want alice", v)
	}
	if v, _ := records[1].Get("age"); v != "25" {
		t.Errorf("record1 age = %v; want 25", v)
	}
}

func TestParseCSVEmpty(t *testing.T) {
	t.Parallel()
	records, err := ParseCSV(nil)
	if err != nil {
		t.Fatalf("ParseCSV(nil) error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("got %d records; want 0", len(records))
	}
}

func TestParseCSVRaggedRowFailsClosed(t *testing.T) {
	t.Parallel()
	_, err := ParseCSV([]byte("a,b\n1\n"))
	if err == nil {
		t.Fatal("expected error on ragged row")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestParseJSONArray(t *testing.T) {
	t.Parallel()
	records, err := ParseJSONArray([]byte(`[{"a":1},{"a":2}]`))
	if err != nil {
		t.Fatalf("ParseJSONArray error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records; want 2", len(records))
	}
}

func TestParseJSONArrayRejectsNonObject(t *testing.T) {
	t.Parallel()
	if _, err := ParseJSONArray([]byte(`[1,2]`)); err == nil {
		t.Fatal("expected error for non-object elements")
	}
	if _, err := ParseJSONArray([]byte(`{"a":1}`)); err == nil {
		t.Fatal("expected error for non-array payload")
	}
}

func TestParseJSONLines(t *testing.T) {
	t.Parallel()
	records, err := ParseJSONLines([]byte("{\"a\":1}\n\n{\"a\":2}\n"))
	if err != nil {
		t.Fatalf("ParseJSONLines error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records; want 2 (blank lines skipped)", len(records))
	}
}

func TestParseJSONLinesRejectsNonObject(t *testing.T) {
	t.Parallel()
	if _, err := ParseJSONLines([]byte("42\n")); err == nil {
		t.Fatal("expected error for non-object line")
	}
}

func TestReadCSVFromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(path, []byte("k,v\nx,1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := ReadCSV(path, payload.DefaultLimits())
	if err != nil {
		t.Fatalf("ReadCSV error: %v", err)
	}
	records, err := stream.Collect(context.Background(), p)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records; want 1", len(records))
	}
}

func TestReadRejectsOversizeFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.jsonl")
	if err := os.WriteFile(path, []byte("{\"a\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadJSONLines(path, payload.Limits{MaxInMemoryBytes: 2})
	if !errors.Is(err, fs.ErrFileTooLarge) {
		t.Fatalf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestReadMissingFile(t *testing.T) {
	t.Parallel()
	_, err := ReadJSONArray(filepath.Join(t.TempDir(), "nope.json"), payload.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
