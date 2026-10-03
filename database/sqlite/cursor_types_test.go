package sqlite_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/database/query"
	apperrors "github.com/kbukum/gokit/errors"
)

type cursorRank int32

type cursorRecord[V any] struct {
	ID   int `gorm:"primaryKey"`
	Rank V   `gorm:"not null"`
}

func (cursorRecord[V]) TableName() string { return "cursor_records" }

func checkCursorRepresentation[V any](t *testing.T, first, second V, supported bool) {
	t.Helper()
	db := newQueryTestDB(t)
	if err := db.AutoMigrate(&cursorRecord[V]{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]cursorRecord[V]{{ID: 1, Rank: first}, {ID: 2, Rank: second}}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := query.CursorConfig{Scope: "records", OrderBy: "rank", UniqueBy: "id"}
	page, err := query.ApplyCursorToGorm[cursorRecord[V]](t.Context(), db, query.CursorParams{PageSize: 1}, cfg)
	if !supported {
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) || page != nil {
			t.Fatalf("expected typed rejection, got %+v, %v", page, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != 1 || page.Pagination.NextCursor == nil {
		t.Fatalf("first page: %+v", page)
	}
	last, err := query.ApplyCursorToGorm[cursorRecord[V]](t.Context(), db,
		query.CursorParams{PageSize: 1, Cursor: *page.Pagination.NextCursor}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Data) != 1 || last.Data[0].ID != 2 || last.Pagination.NextCursor != nil {
		t.Fatalf("last page: %+v", last)
	}
}

func TestCursorScalarRoundTrips(t *testing.T) {
	t.Parallel()
	t.Run("integer", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, int64(-2), int64(3), true)
	})
	t.Run("unsigned", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, uint64(2), uint64(3), true)
	})
	t.Run("named integer", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, cursorRank(2), cursorRank(3), true)
	})
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, "a", "b", true)
	})
	t.Run("time", func(t *testing.T) {
		t.Parallel()
		first := time.Date(2026, time.October, 1, 0, 0, 0, 123456789, time.UTC)
		checkCursorRepresentation(t, first, first.Add(time.Second), true)
	})
}

func TestCursorRejectsNullableRepresentations(t *testing.T) {
	t.Parallel()
	t.Run("integer", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, sql.NullInt64{Int64: 1, Valid: true}, sql.NullInt64{Int64: 2, Valid: true}, false)
	})
	t.Run("string", func(t *testing.T) {
		t.Parallel()
		checkCursorRepresentation(t, sql.NullString{String: "a", Valid: true}, sql.NullString{String: "b", Valid: true}, false)
	})
}
