package sqlite_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/codec"
	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/fs"
)

type listWireRow struct {
	ID uint `gorm:"primaryKey" json:"id"`
}

func TestSharedListFixtures(t *testing.T) {
	t.Parallel()
	data, err := fs.ReadFileLimit("../query/testdata/lists.json", 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := codec.Decode[[]struct {
		Name     string      `json:"name"`
		Mode     string      `json:"mode"`
		Page     int         `json:"page"`
		PageSize int         `json:"pageSize"`
		Expected codec.Value `json:"expected"`
	}](codec.CompactJSON(), string(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := newTestDB(t)
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(ctx, &listWireRow{}); err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Create(&[]listWireRow{{1}, {2}, {3}}).Error; err != nil {
				t.Fatal(err)
			}
			var actual string
			if tc.Mode == "cursor" {
				result, err := query.ApplyCursorToGorm[listWireRow](ctx, db.GormDB.Model(&listWireRow{}), query.CursorParams{PageSize: tc.PageSize}, query.CursorConfig{Scope: "fixtures", OrderBy: "id", UniqueBy: "id"})
				if err != nil {
					t.Fatal(err)
				}
				actual, err = codec.Encode(codec.CompactJSON(), result)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				result, err := query.ApplyToGorm[listWireRow](ctx, db.GormDB.Model(&listWireRow{}), query.Params{Page: tc.Page, PageSize: tc.PageSize}, query.Config{DefaultSort: "id"})
				if err != nil {
					t.Fatal(err)
				}
				actual, err = codec.Encode(codec.CompactJSON(), result)
				if err != nil {
					t.Fatal(err)
				}
			}
			expected, err := codec.CompactJSON().EncodeValue(tc.Expected)
			if err != nil {
				t.Fatal(err)
			}
			if actual != expected {
				t.Fatalf("wire:\n%s\nwant:\n%s", actual, expected)
			}
		})
	}
}
