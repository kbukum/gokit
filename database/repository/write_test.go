package repository

import (
	"context"
	"testing"
)

func TestWriteRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWriteRepository[testModel, string](db, "test")
	ctx := context.Background()

	entity := &testModel{ID: "w1", Name: "Alice", Age: 30}
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify via read
	got, err := repo.GetByID(ctx, "w1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice")
	}
}

func TestWriteRepository_Create_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWriteRepository[testModel, string](db, "test")
	ctx := context.Background()

	entity := &testModel{ID: "dup", Name: "First", Age: 1}
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	dup := &testModel{ID: "dup", Name: "Second", Age: 2}
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestWriteRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWriteRepository[testModel, string](db, "test")
	ctx := context.Background()

	entity := &testModel{ID: "w2", Name: "Bob", Age: 25}
	if err := repo.Create(ctx, entity); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	entity.Name = "Bobby"
	entity.Age = 26
	if err := repo.Update(ctx, entity); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := repo.GetByID(ctx, "w2")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != "Bobby" || got.Age != 26 {
		t.Errorf("got Name=%q Age=%d, want Name=%q Age=%d", got.Name, got.Age, "Bobby", 26)
	}
}

func TestWriteRepository_InheritsRead(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWriteRepository[testModel, string](db, "test")
	ctx := context.Background()

	// Create via write repo, then use read methods
	repo.Create(ctx, &testModel{ID: "w3", Name: "Carol", Age: 30})
	repo.Create(ctx, &testModel{ID: "w4", Name: "Dave", Age: 30})

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}

	results, err := repo.FindAllBy(ctx, "age", 30)
	if err != nil {
		t.Fatalf("FindAllBy failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FindAllBy returned %d, want 2", len(results))
	}
}

func TestWriteRepository_WithTx(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWriteRepository[testModel, string](db, "test")
	ctx := context.Background()

	tx := db.Begin()

	txRepo := repo.WithTx(tx)
	txRepo.Create(ctx, &testModel{ID: "tx1", Name: "InTx", Age: 1})

	// Rollback — should not persist
	tx.Rollback()

	_, err := repo.GetByID(ctx, "tx1")
	if err == nil {
		t.Fatal("expected not-found after rollback")
	}
}
