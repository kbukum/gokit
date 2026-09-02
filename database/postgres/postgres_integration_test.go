//go:build integration

// Postgres adapter integration tests. Gated behind the `integration` build tag so the default
// `go test ./...` stays fast and dependency-free; run with `go test -tags=integration ./...`.
//
// Determinism: each test provisions its own ephemeral PostgreSQL server via testcontainers-go with
// a digest-pinned image, so runs never depend on a developer's or CI runner's local Postgres or on
// a mutable tag. The suite skips only when no Docker provider is healthy; once Docker is reachable,
// any container startup failure fails the test rather than silently skipping.
package postgres_test

import (
	"context"
	"embed"
	"errors"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/migration"
	"github.com/kbukum/gokit/database/postgres"
	"github.com/kbukum/gokit/database/query"
	"github.com/kbukum/gokit/database/repository"
	dbtestutil "github.com/kbukum/gokit/database/testutil"
	"github.com/kbukum/gokit/logging"
)

// postgresImage is pinned by immutable digest so the integration environment is deterministic and
// cannot silently change or run an unreviewed image; bump the digest deliberately. The digest is
// the multi-arch index for postgres:17-alpine, so it resolves on both amd64 and arm64 runners.
const postgresImage = "postgres:17-alpine@sha256:18cfe3ef5e6815560c98237d6216d1e5119702fb0f3894c8785dd58b8bbe5d73"

// setupTimeout bounds container image-pull and startup so a stalled daemon or registry fails the
// test instead of hanging the suite indefinitely.
const setupTimeout = 3 * time.Minute

//go:embed testdata/migrations/*.sql
var migrationsFS embed.FS

// newDSN starts an ephemeral PostgreSQL container and returns a connection DSN. It skips only when
// no Docker provider is healthy; once Docker is reachable, a container startup failure fails the
// test so a bad image or PostgreSQL regression cannot leave CI green.
func newDSN(t *testing.T) string {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("secret"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
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
	dsn := newDSN(t)
	ctx := context.Background()

	reg := database.NewDialectRegistry()
	if err := postgres.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	cfg := database.Config{Enabled: true, DSN: dsn, AutoMigrate: true}
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

	if comp.DB() == nil || !comp.DB().GormDB.Migrator().HasTable(&widget{}) {
		t.Fatal("component did not start and migrate model")
	}
	if health := comp.Health(ctx); health.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v, want healthy", health)
	}
}

// TestRepositoryRoundTrip exercises a full CRUD cycle through the generic repository against
// Postgres, plus a database/testutil fixture load to confirm the shared harness is reusable.
func TestRepositoryRoundTrip(t *testing.T) {
	dsn := newDSN(t)
	ctx := context.Background()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
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
	dsn := newDSN(t)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	cfg := migration.Config{
		DB:     db,
		FS:     migrationsFS,
		Path:   "testdata/migrations",
		Driver: postgres.MigrateDriver(),
	}

	if err := cfg.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	version, dirty, err := cfg.Version()
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != 2 || dirty {
		t.Fatalf("Version = %d dirty=%v, want 2 false", version, dirty)
	}
	if !db.Migrator().HasColumn(&widget{}, "price") {
		t.Fatal("expected price column after Up")
	}

	if err := cfg.Steps(-1); err != nil {
		t.Fatalf("Steps down: %v", err)
	}
	if db.Migrator().HasColumn(&widget{}, "price") {
		t.Fatal("price column should be gone after rolling back one step")
	}

	if err := cfg.Down(); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if db.Migrator().HasTable(&widget{}) {
		t.Fatal("widgets table should be gone after full Down")
	}

	// A second Down is a no-op, not an error.
	if err := cfg.Down(); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("second Down should suppress no-change: %v", err)
	}
}
