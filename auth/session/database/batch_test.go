package database

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/auth/session"
)

func TestSQLLookupBatchReturnsAuthoritativeRecordsAndAbsence(t *testing.T) {
	s, _, clock := fixture(t)
	ctx := context.Background()
	for _, r := range []session.Record{row("first", "one", clock), row("second", "two", clock)} {
		if err := s.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.LookupBatch(ctx, []string{"first", "missing", "second"})
	if err != nil || len(rows) != 2 || rows["first"].Family != "one" || rows["second"].Family != "two" {
		t.Fatalf("batch returned wrong authority: count=%d error=%v", len(rows), err)
	}
	if _, exists := rows["missing"]; exists {
		t.Fatal("missing reference fabricated")
	}
	for _, refs := range [][]string{nil, {"first", "first"}, make([]string, session.MaxLookupBatch+1)} {
		if _, err := s.LookupBatch(ctx, refs); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if rows, err := s.LookupBatch(canceled, []string{"missing"}); err == nil || rows != nil {
		t.Fatal("canceled query became authoritative absence")
	}
}
