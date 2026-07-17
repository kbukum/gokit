package record

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kbukum/gokit/stream"
)

func recordPipeline(recs ...map[string]Value) *stream.Pipeline[Record] {
	items := make([]Record, len(recs))
	for i, r := range recs {
		items[i] = New(r)
	}
	return stream.FromSlice(items)
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := WriteCSV(context.Background(), &buf,
		recordPipeline(map[string]Value{"a": "1", "b": "2"}, map[string]Value{"a": "3", "b": "4"}))
	if err != nil {
		t.Fatalf("WriteCSV error: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d records; want 2", n)
	}
	want := "a,b\n1,2\n3,4\n"
	if buf.String() != want {
		t.Fatalf("CSV = %q; want %q", buf.String(), want)
	}
}

func TestWriteCSVMismatchedColumnsFailsClosed(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	_, err := WriteCSV(context.Background(), &buf,
		recordPipeline(map[string]Value{"a": "1"}, map[string]Value{"b": "2"}))
	if err == nil {
		t.Fatal("expected error on mismatched columns")
	}
}

func TestWriteCSVValueTypes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	_, err := WriteCSV(context.Background(), &buf,
		recordPipeline(map[string]Value{"s": "x", "n": float64(3), "b": true, "o": nil, "m": map[string]Value{"k": "v"}}))
	if err != nil {
		t.Fatalf("WriteCSV error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "true") || !strings.Contains(out, `""k"":""v""`) {
		t.Fatalf("unexpected CSV output: %q", out)
	}
}

func TestWriteJSONArray(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := WriteJSONArray(context.Background(), &buf, recordPipeline(map[string]Value{"a": float64(1)}))
	if err != nil {
		t.Fatalf("WriteJSONArray error: %v", err)
	}
	if n != 1 {
		t.Fatalf("wrote %d; want 1", n)
	}
	if got := strings.TrimSpace(buf.String()); got != `[{"a":1}]` {
		t.Fatalf("JSON array = %q", got)
	}
}

func TestWriteJSONLines(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := WriteJSONLines(context.Background(), &buf,
		recordPipeline(map[string]Value{"a": float64(1)}, map[string]Value{"a": float64(2)}))
	if err != nil {
		t.Fatalf("WriteJSONLines error: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d; want 2", n)
	}
	want := "{\"a\":1}\n{\"a\":2}\n"
	if buf.String() != want {
		t.Fatalf("JSONL = %q; want %q", buf.String(), want)
	}
}

func TestWriteRoundTripCSV(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if _, err := WriteCSV(context.Background(), &buf,
		recordPipeline(map[string]Value{"k": "v", "n": "1"})); err != nil {
		t.Fatal(err)
	}
	records, err := ParseCSV(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseCSV error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("round-trip produced %d records; want 1", len(records))
	}
	if v, _ := records[0].Get("k"); v != "v" {
		t.Fatalf("round-trip k = %v; want v", v)
	}
}
