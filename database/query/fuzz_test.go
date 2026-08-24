package query

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// FuzzParseFromRequest asserts request parsing never panics on arbitrary query strings and only
// ever emits conditions whose field is allow-listed and whose operator is valid — the two
// invariants the GORM builder relies on downstream. ParseFromRequest is a trust boundary (raw HTTP
// input), so a produced condition that escapes the allow-list or carries an invalid operator would
// reach query construction unchecked.
func FuzzParseFromRequest(f *testing.F) {
	for _, raw := range []string{
		"", "page=2&limit=5", "status=eq.active", "filter=status=eq.active&priority=gt.3",
		"tags=in.(a,b,c)", "name=is.null", "search=hello", "limit=-1", "sortBy=name&order=desc",
	} {
		f.Add(raw)
	}
	cfg := Config{AllowedFilters: []string{"status", "priority", "tags", "name"}}
	f.Fuzz(func(t *testing.T, raw string) {
		req := httptest.NewRequest("GET", "/", http.NoBody)
		req.URL.RawQuery = raw
		params := ParseFromRequest(req, cfg)
		if params.PageSize < 1 || params.PageSize > cfg.maxPageSize() {
			t.Fatalf("PageSize %d outside [1,%d]", params.PageSize, cfg.maxPageSize())
		}
		for _, cond := range params.Query.Conditions {
			if !isFieldAllowed(cond.Field, cfg.AllowedFilters) {
				t.Fatalf("condition on disallowed field %q", cond.Field)
			}
			if !cond.Operator.IsValid() {
				t.Fatalf("condition with invalid operator %q", cond.Operator)
			}
		}
	})
}
