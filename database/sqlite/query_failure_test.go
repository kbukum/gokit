package sqlite_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/database/query"
)

func TestListSurfacesInvalidFiltersAndFacetOverflow(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	for _, cond := range []query.Condition{
		{Field: "name; --", Operator: query.OpEq, Value: "none"},
		{Field: "name", Operator: "unsupported", Value: "none"},
	} {
		if _, err := query.ApplyToGorm[widget](context.Background(), db.Model(&widget{}), query.Params{Query: query.FilterQuery{Conditions: []query.Condition{cond}}}, query.Config{}); err == nil {
			t.Fatal("invalid filter silently broadened the query")
		}
	}
	if _, err := query.ApplyToGorm[widget](context.Background(), db.Model(&widget{}), query.Params{}, query.Config{FacetFields: []string{"category"}, MaxFacetValues: 1}); err == nil {
		t.Fatal("expected facet overflow")
	}
}
