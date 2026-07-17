package record

import (
	"testing"
)

func TestRecordSelectAndGet(t *testing.T) {
	t.Parallel()
	rec := New(map[string]Value{"a": "1", "b": "2", "c": "3"})

	if v, ok := rec.Get("b"); !ok || v != "2" {
		t.Fatalf("Get(b) = %v, %v; want 2, true", v, ok)
	}
	if _, ok := rec.Get("missing"); ok {
		t.Fatalf("Get(missing) should report absent")
	}
	if rec.Len() != 3 {
		t.Fatalf("Len = %d; want 3", rec.Len())
	}

	sel := rec.Select([]string{"a", "c", "missing"})
	if sel.Len() != 2 {
		t.Fatalf("Select length = %d; want 2", sel.Len())
	}
	if _, ok := sel.Get("b"); ok {
		t.Fatalf("Select should drop unlisted column b")
	}
}

func TestRecordKeysSorted(t *testing.T) {
	t.Parallel()
	rec := New(map[string]Value{"z": 1, "a": 2, "m": 3})
	keys := rec.Keys()
	want := []string{"a", "m", "z"}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("Keys = %v; want %v", keys, want)
		}
	}
}

func TestRecordIsCopyIsolated(t *testing.T) {
	t.Parallel()
	src := map[string]Value{"a": "1"}
	rec := New(src)
	src["a"] = "mutated"
	if v, _ := rec.Get("a"); v != "1" {
		t.Fatalf("record should not observe caller mutation, got %v", v)
	}

	fields := rec.Fields()
	fields["a"] = "changed"
	if v, _ := rec.Get("a"); v != "1" {
		t.Fatalf("Fields() must return a copy, record mutated to %v", v)
	}
}

func TestFormatString(t *testing.T) {
	t.Parallel()
	cases := map[Format]string{
		FormatCSV:       "csv",
		FormatJSONArray: "json_array",
		FormatJSONLines: "json_lines",
		Format(99):      "unknown",
	}
	for f, want := range cases {
		if got := f.String(); got != want {
			t.Errorf("Format(%d).String() = %q; want %q", f, got, want)
		}
	}
}
