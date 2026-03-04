package repository

import (
	"context"
	"testing"
)

func TestReadRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")
	seedModel(t, db, "r1", "Alice", 30)

	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetByID(ctx, "r1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "Alice" {
			t.Errorf("Name = %q, want %q", got.Name, "Alice")
		}
	})

	t.Run("not found returns nil", func(t *testing.T) {
		got, err := repo.GetByID(ctx, "missing")
		if err != nil {
			t.Fatalf("expected nil error for not-found, got: %v", err)
		}
		if got != nil {
			t.Fatal("expected nil result for not-found")
		}
	})
}

func TestReadRepository_FindOneBy(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")
	seedModel(t, db, "r1", "Alice", 30)

	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		got, err := repo.FindOneBy(ctx, "name", "Alice")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "r1" {
			t.Errorf("ID = %q, want %q", got.ID, "r1")
		}
	})

	t.Run("not found returns nil", func(t *testing.T) {
		got, err := repo.FindOneBy(ctx, "name", "Nobody")
		if err != nil {
			t.Fatalf("expected nil error for not-found, got: %v", err)
		}
		if got != nil {
			t.Fatal("expected nil result for not-found")
		}
	})
}

func TestReadRepository_FindAllBy(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")
	seedModel(t, db, "r1", "Alice", 30)
	seedModel(t, db, "r2", "Bob", 30)
	seedModel(t, db, "r3", "Carol", 25)

	ctx := context.Background()

	got, err := repo.FindAllBy(ctx, "age", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d results, want 2", len(got))
	}
}

func TestReadRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")
	seedModel(t, db, "r1", "Alice", 30)
	seedModel(t, db, "r2", "Bob", 25)

	ctx := context.Background()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}
}

func TestReadRepository_DB(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")
	if repo.DB() == nil {
		t.Error("DB() should not be nil")
	}
}

func TestReadRepository_Resource(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "my_resource")
	if got := repo.Resource(); got != "my_resource" {
		t.Errorf("Resource() = %q, want %q", got, "my_resource")
	}
}

func TestReadRepository_WithTx(t *testing.T) {
	db := setupTestDB(t)
	repo := NewReadRepository[testModel, string](db, "test")

	tx := db.Begin()
	defer tx.Rollback()

	txRepo := repo.WithTx(tx)
	if txRepo.DB() == repo.DB() {
		t.Error("WithTx should return repo with different DB")
	}
	if txRepo.Resource() != repo.Resource() {
		t.Error("WithTx should preserve resource name")
	}
}

func TestReadRepository_WithIDField(t *testing.T) {
	db := setupTestDB(t)
	// Use custom ID field name (still maps to the "id" column via gorm tag,
	// but tests that WithIDField option is wired correctly)
	repo := NewReadRepository[testModel, string](db, "test", WithIDField("id"))
	seedModel(t, db, "custom1", "Test", 1)

	ctx := context.Background()
	got, err := repo.GetByID(ctx, "custom1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Test" {
		t.Errorf("Name = %q, want %q", got.Name, "Test")
	}
}
