package resilience

import "testing"

func newBulkhead(t testing.TB, cfg BulkheadConfig) *Bulkhead {
	t.Helper()
	b, err := NewBulkhead(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
