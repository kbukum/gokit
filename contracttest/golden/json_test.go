package golden

import (
	"fmt"
	"testing"
)

// recorder captures Fatalf calls so failure paths can be asserted without
// aborting the enclosing test. Embedding testing.TB satisfies its unexported
// method; only Helper and Fatalf are exercised by the golden assertions.
type recorder struct {
	testing.TB
	failed bool
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) {
	_ = fmt.Sprintf(format, args...)
	r.failed = true
}

func TestAssertJSONMatchesRegardlessOfFormatting(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	AssertJSON(rec, []byte("{\n  \"b\": 1,\n  \"a\": 2\n}"), `{"a":2,"b":1}`)
	if rec.failed {
		t.Fatalf("expected canonically equal JSON to match")
	}
}

func TestAssertJSONRejectsTrailingContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
	}{
		{"trailing object", `{"valid":true}{"extra":1}`},
		{"trailing close bracket", `{}]`},
		{"trailing close brace", `{}}`},
		{"trailing scalar", `1 2`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			AssertJSON(rec, []byte(tc.got), tc.got)
			if !rec.failed {
				t.Fatalf("expected trailing content %q to be rejected", tc.got)
			}
		})
	}
}

func TestAssertJSONPreservesLargeIntegerContracts(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	AssertJSON(rec, []byte("9007199254740992"), "9007199254740993")
	if !rec.failed {
		t.Fatalf("expected distinct large integers to differ")
	}
}
