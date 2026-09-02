package sqlite_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
)

type registryModel struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func TestDialectRegistryRegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := database.NewDialectRegistry()
	if err := reg.Register(sqlite.Dialect()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get("sqlite"); !ok {
		t.Fatal("registered driver not found")
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("unregistered driver should not be found")
	}
}

func TestComponentStartFromRegistryAndMigrates(t *testing.T) {
	ctx := context.Background()
	reg := database.NewDialectRegistry()
	if err := reg.Register(sqlite.Dialect()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	cfg := database.Config{Enabled: true, DSN: ":memory:", AutoMigrate: true}
	cfg.ApplyDefaults()
	comp := database.NewComponent(cfg, logging.NewDefault("test")).
		WithDialectFromRegistry(reg, "sqlite").
		WithAutoMigrate(&registryModel{})
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if comp.DB() == nil || !comp.DB().GormDB.Migrator().HasTable(&registryModel{}) {
		t.Fatal("component did not start and migrate model")
	}
	if health := comp.Health(ctx); health.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v, want healthy", health)
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if health := comp.Health(ctx); health.Status != component.StatusUnhealthy {
		t.Fatalf("closed Health = %+v, want unhealthy", health)
	}
}

func TestComponentStartFailsForRegistryDriverErrors(t *testing.T) {
	ctx := context.Background()
	cfg := database.Config{Enabled: true, DSN: ":memory:"}
	cfg.ApplyDefaults()
	log := logging.NewDefault("test")

	if err := database.NewComponent(cfg, log).WithDialectFromRegistry(nil, "sqlite").Start(ctx); err == nil {
		t.Fatal("expected error for nil registry")
	}
	if err := database.NewComponent(cfg, log).
		WithDialectFromRegistry(database.NewDialectRegistry(), "sqlite").Start(ctx); err == nil {
		t.Fatal("expected error for unregistered driver")
	}
}
