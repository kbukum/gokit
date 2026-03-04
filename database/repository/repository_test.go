package repository

import (
	"context"
	"testing"
)

func TestRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	repo.Create(ctx, &testModel{ID: "d1", Name: "ToDelete", Age: 1})

	if err := repo.Delete(ctx, "d1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := repo.GetByID(ctx, "d1")
	if err == nil {
		t.Fatal("expected not-found after Delete")
	}
}

func TestRepository_Delete_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	// Deleting a non-existent record should not error in GORM (affected rows = 0)
	if err := repo.Delete(ctx, "nope"); err != nil {
		t.Fatalf("Delete of non-existent should not error, got: %v", err)
	}
}

func TestRepository_FullCRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	// Create
	entity := &testModel{ID: "crud1", Name: "Alice", Age: 30}
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read
	got, err := repo.GetByID(ctx, "crud1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("after create: Name = %q, want %q", got.Name, "Alice")
	}

	// Update
	got.Name = "Alicia"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := repo.GetByID(ctx, "crud1")
	if updated.Name != "Alicia" {
		t.Errorf("after update: Name = %q, want %q", updated.Name, "Alicia")
	}

	// Count
	count, _ := repo.Count(ctx)
	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}

	// Delete
	if err := repo.Delete(ctx, "crud1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	count, _ = repo.Count(ctx)
	if count != 0 {
		t.Errorf("after delete: Count = %d, want 0", count)
	}
}

func TestRepository_InheritsWriteAndRead(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	// All three tiers accessible
	repo.Create(ctx, &testModel{ID: "i1", Name: "A", Age: 1})
	repo.Create(ctx, &testModel{ID: "i2", Name: "B", Age: 1})

	// Read methods (from ReadRepository)
	got, err := repo.FindOneBy(ctx, "name", "A")
	if err != nil {
		t.Fatalf("FindOneBy: %v", err)
	}
	if got.ID != "i1" {
		t.Errorf("FindOneBy ID = %q, want %q", got.ID, "i1")
	}

	all, err := repo.FindAllBy(ctx, "age", 1)
	if err != nil {
		t.Fatalf("FindAllBy: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("FindAllBy len = %d, want 2", len(all))
	}

	// Write methods (from WriteRepository)
	got.Name = "Updated"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Delete (from Repository)
	if err := repo.Delete(ctx, "i2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	count, _ := repo.Count(ctx)
	if count != 1 {
		t.Errorf("after delete: Count = %d, want 1", count)
	}
}

func TestRepository_WithTx_Commit(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	tx := db.Begin()
	txRepo := repo.WithTx(tx)
	txRepo.Create(ctx, &testModel{ID: "tc1", Name: "Committed", Age: 1})
	tx.Commit()

	// Should be visible after commit
	got, err := repo.GetByID(ctx, "tc1")
	if err != nil {
		t.Fatalf("expected record after commit: %v", err)
	}
	if got.Name != "Committed" {
		t.Errorf("Name = %q, want %q", got.Name, "Committed")
	}
}

func TestRepository_WithTx_Rollback(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository[testModel, string](db, "test")
	ctx := context.Background()

	tx := db.Begin()
	txRepo := repo.WithTx(tx)
	txRepo.Create(ctx, &testModel{ID: "tr1", Name: "RolledBack", Age: 1})
	tx.Rollback()

	_, err := repo.GetByID(ctx, "tr1")
	if err == nil {
		t.Fatal("expected not-found after rollback")
	}
}
