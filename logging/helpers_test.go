package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// newBufferedLogger builds a Logger whose default sink writes to buf, so tests
// can assert on the real rendered output without touching stdout. It exercises
// the full pipeline (sink, schema normalization, middleware) rather than a
// stand-in fake.
func newBufferedLogger(buf *bytes.Buffer, level, format string) *Logger {
	cfg := &Config{Level: level, Format: format, Output: OutputStdout(), Timestamp: true}
	l, err := New(cfg, "test", WithWriter(buf))
	if err != nil {
		panic(err)
	}
	return l
}

// decodeLines parses each non-empty JSON line in buf into a map.
func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}
