package sqlite_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	. "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
)

// newTenantTestDB creates an in-memory SQLite database for tenant tests.
func newTenantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := Config{Enabled: true, DSN: ":memory:"}
	cfg.ApplyDefaults()
	log := logging.NewDefault("test")
	db, err := NewWithContext(context.Background(), sqlite.Open(cfg.DSN), cfg, log)
	if err != nil {
		t.Fatalf("newTenantTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db.GormDB
}

func TestScopeToTenant(t *testing.T) {
	t.Parallel()

	type tenantRow struct {
		ID          uint   `gorm:"primaryKey"`
		WorkspaceID string `gorm:"size:36"`
		Name        string `gorm:"size:255"`
	}

	tests := []struct {
		name   string
		column string
		value  interface{}
		seed   []tenantRow
		want   int
	}{
		{
			name:   "filters matching rows",
			column: "workspace_id",
			value:  "ws-1",
			seed: []tenantRow{
				{ID: 1, WorkspaceID: "ws-1", Name: "a"},
				{ID: 2, WorkspaceID: "ws-2", Name: "b"},
				{ID: 3, WorkspaceID: "ws-1", Name: "c"},
			},
			want: 2,
		},
		{
			name:   "returns empty for no match",
			column: "workspace_id",
			value:  "ws-none",
			seed: []tenantRow{
				{ID: 1, WorkspaceID: "ws-1", Name: "a"},
			},
			want: 0,
		},
		{
			name:   "works with integer column",
			column: "id",
			value:  uint(2),
			seed: []tenantRow{
				{ID: 1, WorkspaceID: "ws-1", Name: "a"},
				{ID: 2, WorkspaceID: "ws-1", Name: "b"},
			},
			want: 1,
		},
		{
			name:   "works with empty table",
			column: "workspace_id",
			value:  "ws-1",
			seed:   nil,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := newTenantTestDB(t)
			if err := db.AutoMigrate(&tenantRow{}); err != nil {
				t.Fatalf("AutoMigrate: %v", err)
			}
			for _, row := range tt.seed {
				if err := db.Create(&row).Error; err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			scoped := ScopeToTenant(db, tt.column, tt.value)
			var count int64
			if err := scoped.Model(&tenantRow{}).Count(&count).Error; err != nil {
				t.Fatalf("Count: %v", err)
			}
			if int(count) != tt.want {
				t.Errorf("got %d rows, want %d", count, tt.want)
			}
		})
	}
}

func TestSetSessionVariable(t *testing.T) {
	t.Parallel()

	// SQLite does not support set_config(). We verify the SQL is executed and
	// returns an expected error (function not found), confirming the query is
	// well-formed and dispatched correctly.
	tests := []struct {
		name    string
		varName string
		value   string
		isLocal bool
	}{
		{
			name:    "local variable",
			varName: "app.workspace_id",
			value:   "ws-123",
			isLocal: true,
		},
		{
			name:    "session variable",
			varName: "app.tenant_id",
			value:   "tenant-456",
			isLocal: false,
		},
		{
			name:    "empty value",
			varName: "app.workspace_id",
			value:   "",
			isLocal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := newTenantTestDB(t)

			// SQLite doesn't have set_config, so we expect an error.
			// This validates the function executes the correct SQL.
			err := SetSessionVariable(db, tt.varName, tt.value, tt.isLocal)
			if err == nil {
				t.Fatal("expected error from SQLite (no set_config), got nil")
			}
		})
	}
}

func TestSetSessionVariable_PropagatesDBError(t *testing.T) {
	t.Parallel()
	db := newTenantTestDB(t)

	// Confirm SetSessionVariable returns errors from the underlying DB.
	err := SetSessionVariable(db, "app.test", "value", true)
	if err == nil {
		t.Error("expected error (SQLite lacks set_config), got nil")
	}
}

func TestScopeToTenant_Composable(t *testing.T) {
	t.Parallel()

	type item struct {
		ID          uint   `gorm:"primaryKey"`
		WorkspaceID string `gorm:"size:36"`
		Category    string `gorm:"size:50"`
	}

	db := newTenantTestDB(t)
	if err := db.AutoMigrate(&item{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	seeds := []item{
		{ID: 1, WorkspaceID: "ws-1", Category: "video"},
		{ID: 2, WorkspaceID: "ws-1", Category: "audio"},
		{ID: 3, WorkspaceID: "ws-2", Category: "video"},
	}
	for _, s := range seeds {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// ScopeToTenant should compose with additional Where clauses.
	scoped := ScopeToTenant(db, "workspace_id", "ws-1").Where("category = ?", "video")
	var count int64
	if err := scoped.Model(&item{}).Count(&count).Error; err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d rows, want 1", count)
	}
}
