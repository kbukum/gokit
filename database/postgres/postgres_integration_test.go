//go:build integration

// Postgres adapter integration tests. Gated behind the `integration` build tag so the default
// `go test ./...` stays fast and dependency-free; run with `go test -tags=integration ./...`.
//
// Determinism: each test provisions its own ephemeral PostgreSQL server via testcontainers-go with
// a digest-pinned image, so runs never depend on a developer's or CI runner's local Postgres or on
// a mutable tag. Missing Docker, startup failure, and termination failure all fail this required gate.
package postgres_test

import (
	"context"
	"embed"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	pgtest "github.com/kbukum/gokit/database/postgres/testutil"
	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/repository"
	dbtestutil "github.com/kbukum/gokit/database/testutil"
	"github.com/kbukum/gokit/logging"
)

//go:embed testdata/migrations/*.sql
var migrationsFS embed.FS

// newDSN consumes the adapter-owned fixture; Docker and successful termination are required.
func newParams(t *testing.T) database.ConnParams {
	t.Helper()
	fixture, err := pgtest.Start(t.Context())
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fixture.Close(ctx); err != nil {
			t.Errorf("terminate container: %v", err)
		}
	})
	return fixture.Params
}

type widget struct {
	ID    uint   `gorm:"primaryKey"`
	Name  string `gorm:"size:255"`
	Price int64
}

func (widget) TableName() string { return "widgets" }

// TestComponentStartFromRegistryAndMigrates proves the registry seam works end to end against a
// real server: register the driver, start the component from the registry, and auto-migrate.
func TestComponentStartFromRegistryAndMigrates(t *testing.T) {
	params := newParams(t)
	ctx := context.Background()

	reg := database.NewDialectRegistry()
	if err := postgres.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	cfg := database.Config{Enabled: true, Params: params, AutoMigrate: true}
	cfg.ApplyDefaults()
	comp := database.NewComponent(cfg, logging.NewDefault("test")).
		WithDialectFromRegistry(reg, postgres.Name).
		WithAutoMigrate(&widget{})
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := comp.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	if db, err := comp.DB(); err != nil || !db.GormDB.Migrator().HasTable(&widget{}) {
		t.Fatal("component did not start and migrate model")
	}
	if health := comp.Health(ctx); health.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v, want healthy", health)
	}
}

// TestRepositoryRoundTrip exercises a full CRUD cycle through the generic repository against
// Postgres, plus a database/testutil fixture load to confirm the shared harness is reusable.
func TestRepositoryRoundTrip(t *testing.T) {
	params := newParams(t)
	ctx := context.Background()

	db, err := gorm.Open(preparedDialector(t, params), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&widget{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := repository.NewRepository[widget, uint](db, "widget")
	if err := repo.Create(ctx, &widget{ID: 1, Name: "gadget", Price: 10}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, 1)
	if err != nil || got == nil {
		t.Fatalf("GetByID = %+v err=%v", got, err)
	}
	if got.Name != "gadget" {
		t.Fatalf("Name = %q, want gadget", got.Name)
	}
	got.Name = "gizmo"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(ctx, 1)
	if err != nil || updated.Name != "gizmo" {
		t.Fatalf("after update Name = %q err=%v", updated.Name, err)
	}

	dbtestutil.MustLoadFixture(t, db, "widgets", []map[string]any{
		{"id": 2, "name": "sprocket", "price": 5},
	})
	dbtestutil.AssertRowCount(t, db, "widgets", 2)

	if err := repo.Delete(ctx, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("Count = %d, want 1", count)
	}
	if _, err := repo.List(ctx, query.Params{Page: 1, PageSize: 10}, query.Config{}); err != nil {
		t.Fatalf("List: %v", err)
	}
}

// TestMigrationsUpAndDown drives golang-migrate through postgres.MigrateDriver against the
// ephemeral server, proving forward and rollback migrations apply symmetrically.
func TestMigrationsUpAndDown(t *testing.T) {
	params := newParams(t)

	db, err := gorm.Open(preparedDialector(t, params), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	cfg := migration.Config{
		DB:     db,
		FS:     migrationsFS,
		Path:   "testdata/migrations",
		Driver: postgres.MigrateDriver(),
		Table:  migration.Table{Schema: "fixture"},
	}

	if err := cfg.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	version, dirty, err := cfg.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("Version = %d dirty=%v, want 2 false", version, dirty)
	}
	if !db.Migrator().HasColumn(&widget{}, "price") {
		t.Fatal("expected price column after Up")
	}

	if err := cfg.Steps(context.Background(), -1); err != nil {
		t.Fatalf("Steps down: %v", err)
	}
	if db.Migrator().HasColumn(&widget{}, "price") {
		t.Fatal("price column should be gone after rolling back one step")
	}

	if err := cfg.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if db.Migrator().HasTable(&widget{}) {
		t.Fatal("widgets table should be gone after full Down")
	}

	// A second Down is a no-op, not an error.
	if err := cfg.Down(context.Background()); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("second Down should suppress no-change: %v", err)
	}
}
