package sqlite_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/repository"
	"github.com/kbukum/gokit/database/types"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestCursorStableAndBoundToQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newQueryTestDB(t)
	cfg := query.CursorConfig{Config: query.Config{MaxPageSize: 2}, Scope: "workspace:one/widgets", OrderBy: "category", UniqueBy: "id"}
	first, err := query.ApplyCursorToGorm[widget](ctx, db.Model(&widget{}), query.CursorParams{PageSize: 3}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Data) != 2 || first.Pagination.PageSize != 2 || first.Pagination.NextCursor == nil {
		t.Fatalf("first page: %+v", first)
	}
	if err := db.Create(&widget{Name: "insert-before", Category: "a", OwnerID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	second, err := query.ApplyCursorToGorm[widget](ctx, db.Model(&widget{}), query.CursorParams{PageSize: 2, Cursor: *first.Pagination.NextCursor}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Data) != 2 || second.Pagination.NextCursor != nil {
		t.Fatalf("last page: %+v", second)
	}
	got := []uint{first.Data[0].ID, first.Data[1].ID, second.Data[0].ID, second.Data[1].ID}
	if !reflect.DeepEqual(got, []uint{1, 2, 3, 4}) {
		t.Fatalf("traversal skipped or repeated rows: %v", got)
	}
	for _, change := range []string{"scope", "filter", "order", "malformed"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			changed := cfg
			params := query.CursorParams{Cursor: *first.Pagination.NextCursor}
			switch change {
			case "scope":
				changed.Scope = "workspace:two/widgets"
			case "filter":
				params.Query.Conditions = []query.Condition{{Field: "name", Operator: query.OpEq, Value: "alpha"}}
			case "order":
				changed.Descending = true
			case "malformed":
				params.Cursor = "not-a-cursor"
			}
			if _, err := query.ApplyCursorToGorm[widget](ctx, db.Model(&widget{}), params, changed); err == nil {
				t.Fatal("expected invalid cursor")
			}
		})
	}
}

func TestCursorEmptyDescendingAndCancellation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newQueryTestDB(t)
	cfg := query.CursorConfig{Scope: "widgets", OrderBy: "price", UniqueBy: "id", Descending: true}
	params := query.CursorParams{PageSize: 2}
	var ids []uint
	for {
		page, err := query.ApplyCursorToGorm[widget](ctx, db.Model(&widget{}), params, cfg)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Data {
			ids = append(ids, row.ID)
		}
		if page.Pagination.NextCursor == nil {
			break
		}
		params.Cursor = *page.Pagination.NextCursor
	}
	if !reflect.DeepEqual(ids, []uint{4, 3, 2, 1}) {
		t.Fatal(ids)
	}
	params = query.CursorParams{Query: query.FilterQuery{Conditions: []query.Condition{{Field: "name", Operator: query.OpEq, Value: "absent"}}}}
	page, err := query.ApplyCursorToGorm[widget](ctx, db.Model(&widget{}), params, cfg)
	if err != nil || page.Data == nil || len(page.Data) != 0 || page.Pagination.NextCursor != nil {
		t.Fatalf("empty page: %+v %v", page, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := query.ApplyCursorToGorm[widget](canceled, db.Model(&widget{}), query.CursorParams{}, cfg); err == nil {
		t.Fatal("expected cancellation")
	}
}

type orderedRecord struct {
	ID    int `gorm:"primaryKey"`
	Order int
}

func TestCursorQuotesAndQualifiesColumns(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	if err := db.AutoMigrate(&orderedRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]orderedRecord{{ID: 1, Order: 1}, {ID: 2, Order: 1}, {ID: 3, Order: 2}}).Error; err != nil {
		t.Fatal(err)
	}
	base := db.Model(&orderedRecord{}).Joins("JOIN ordered_records AS other ON other.id = ordered_records.id").Select("ordered_records.*")
	cfg := query.CursorConfig{Scope: "records", OrderBy: "order", UniqueBy: "id"}
	params := query.CursorParams{PageSize: 1}
	for _, id := range []int{1, 2, 3} {
		page, err := query.ApplyCursorToGorm[orderedRecord](t.Context(), base, params, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Data) != 1 || page.Data[0].ID != id {
			t.Fatalf("unexpected page: %+v", page)
		}
		if page.Pagination.NextCursor != nil {
			params.Cursor = *page.Pagination.NextCursor
		}
	}
}

type canonicalRecord struct {
	types.BaseModel
	Name string
}

func TestCursorSupportsCanonicalUUIDModel(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	if err := db.Exec("CREATE TABLE canonical_records(id TEXT PRIMARY KEY, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, name TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	for _, id := range ids {
		if err := db.Create(&canonicalRecord{BaseModel: types.BaseModel{ID: id}, Name: "same"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := repository.NewReadRepository[canonicalRecord, uuid.UUID](db, "record")
	cfg := query.CursorConfig{Scope: "records", OrderBy: "name", UniqueBy: "id"}
	first, err := repo.ListCursor(t.Context(), query.CursorParams{PageSize: 1}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Pagination.NextCursor == nil || first.Data[0].ID != ids[0] {
		t.Fatalf("unexpected first page: %+v", first)
	}
	last, err := repo.ListCursor(t.Context(), query.CursorParams{PageSize: 1, Cursor: *first.Pagination.NextCursor}, cfg)
	if err != nil || len(last.Data) != 1 || last.Data[0].ID != ids[1] || last.Pagination.NextCursor != nil {
		t.Fatalf("unexpected last page: %+v, %v", last, err)
	}
}

func TestCursorRepositoryTranslatesErrors(t *testing.T) {
	t.Parallel()
	db := newQueryTestDB(t)
	repo := repository.NewReadRepository[widget, uint](db, "widget")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := repo.ListCursor(ctx, query.CursorParams{}, query.CursorConfig{Scope: "widgets", OrderBy: "id", UniqueBy: "id"})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected typed cancellation, got %v", err)
	}
}
