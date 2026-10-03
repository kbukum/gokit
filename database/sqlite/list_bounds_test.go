package sqlite_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/kbukum/gokit/database/query"
)

func TestDirectListBounds(t *testing.T) {
	t.Parallel()
	for _, size := range []int{-1, 0, 2, 3, 1000000} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			db := newQueryTestDB(t)
			result, err := query.ApplyToGorm[widget](context.Background(), db.Model(&widget{}), query.Params{Page: 1, PageSize: size}, query.Config{DefaultPageSize: 2, MaxPageSize: 2})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Data) != 2 || result.Pagination.PageSize != 2 {
				t.Fatalf("size %d: got %d rows, pagination %+v", size, len(result.Data), result.Pagination)
			}
		})
	}
}
