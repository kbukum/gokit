package golden

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// AssertJSON compares two JSON documents after canonical decoding and compact encoding.
func AssertJSON(t testing.TB, got []byte, want string) {
	t.Helper()

	gotCanonical := canonicalJSON(t, got)
	wantCanonical := canonicalJSON(t, []byte(want))
	if gotCanonical != wantCanonical {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", gotCanonical, wantCanonical)
	}
}

func canonicalJSON(t testing.TB, data []byte) string {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		t.Fatalf("invalid JSON %q: %v", strings.TrimSpace(string(data)), err)
	}
	if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		t.Fatalf("invalid JSON %q: unexpected trailing content after first value", strings.TrimSpace(string(data)))
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	return string(out)
}
