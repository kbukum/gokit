package vectorstore

import (
	"context"
	"math"
	"testing"
)

func TestInMemoryStoreEnsureCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 3)
	if err != nil {
		t.Errorf("EnsureCollection() error = %v", err)
	}

	// Should not error when called again
	err = store.EnsureCollection(ctx, "test", 3)
	if err != nil {
		t.Errorf("EnsureCollection() second call error = %v", err)
	}
}

func TestInMemoryStoreUpsertAndSearch(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 3)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	payload1 := NewPointPayload().WithField("name", "doc1")
	err = store.Upsert(ctx, "test", "1", []float32{1.0, 0.0, 0.0}, payload1)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	payload2 := NewPointPayload().WithField("name", "doc2")
	err = store.Upsert(ctx, "test", "2", []float32{0.0, 1.0, 0.0}, payload2)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", []float32{1.0, 0.0, 0.0}, 10, nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}

	if results[0].ID != "1" {
		t.Errorf("Search() first result ID = %s, want 1", results[0].ID)
	}

	if math.Abs(float64(results[0].Score-1.0)) > 1e-6 {
		t.Errorf("Search() first result score = %v, want ~1.0", results[0].Score)
	}
}

func TestInMemoryStoreUpsertUpdatesExisting(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 2)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	payload1 := NewPointPayload().WithField("v", "old")
	err = store.Upsert(ctx, "test", "1", []float32{1.0, 0.0}, payload1)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	payload2 := NewPointPayload().WithField("v", "new")
	err = store.Upsert(ctx, "test", "1", []float32{0.0, 1.0}, payload2)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", []float32{0.0, 1.0}, 10, nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Search() returned %d results, want 1", len(results))
	}

	if results[0].ID != "1" {
		t.Errorf("Search() result ID = %s, want 1", results[0].ID)
	}

	if v, ok := results[0].Payload.Fields["v"].(string); !ok || v != "new" {
		t.Errorf("Search() payload field v = %v, want 'new'", results[0].Payload.Fields["v"])
	}
}

func TestInMemoryStoreSearchWithFilter(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 2)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	err = store.Upsert(
		ctx,
		"test",
		"1",
		[]float32{1.0, 0.0},
		NewPointPayload().WithField("type", "a"),
	)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	err = store.Upsert(
		ctx,
		"test",
		"2",
		[]float32{1.0, 0.0},
		NewPointPayload().WithField("type", "b"),
	)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	filter := NewSearchFilter().MustMatch("type", "a")
	results, err := store.Search(ctx, "test", []float32{1.0, 0.0}, 10, filter)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Search() returned %d results, want 1", len(results))
	}

	if results[0].ID != "1" {
		t.Errorf("Search() result ID = %s, want 1", results[0].ID)
	}
}

func TestInMemoryStoreDelete(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 2)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	err = store.Upsert(ctx, "test", "1", []float32{1.0, 0.0}, NewPointPayload())
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	err = store.Delete(ctx, "test", "1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	results, err := store.Search(ctx, "test", []float32{1.0, 0.0}, 10, nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Search() returned %d results after delete, want 0", len(results))
	}
}

func TestInMemoryStoreUpsertWrongDimensions(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 3)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	err = store.Upsert(ctx, "test", "1", []float32{1.0, 0.0}, NewPointPayload())
	if err == nil {
		t.Error("Upsert() expected error for dimension mismatch")
	}
}

func TestInMemoryStoreUpsertMissingCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.Upsert(ctx, "nonexistent", "1", []float32{1.0}, NewPointPayload())
	if err == nil {
		t.Error("Upsert() expected error for missing collection")
	}
}

func TestInMemoryStoreSearchMissingCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	_, err := store.Search(ctx, "nonexistent", []float32{1.0}, 10, nil)
	if err == nil {
		t.Error("Search() expected error for missing collection")
	}
}

func TestInMemoryStoreDeleteMissingCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent", "1")
	if err == nil {
		t.Error("Delete() expected error for missing collection")
	}
}

func TestInMemoryStoreSearchLimit(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.EnsureCollection(ctx, "test", 1)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}

	// Insert 5 points
	for i := 0; i < 5; i++ {
		payload := NewPointPayload().WithField("index", i)
		err = store.Upsert(ctx, "test", string(rune('1'+i)), []float32{float32(i)}, payload)
		if err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	// Search with limit
	results, err := store.Search(ctx, "test", []float32{4.0}, 2, nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}
}

func TestPointPayloadWithField(t *testing.T) {
	payload := NewPointPayload().
		WithField("name", "test").
		WithField("count", 42).
		WithField("active", true)

	if v, ok := payload.Fields["name"].(string); !ok || v != "test" {
		t.Errorf("payload.Fields[name] = %v, want 'test'", payload.Fields["name"])
	}

	if v, ok := payload.Fields["count"].(int); !ok || v != 42 {
		t.Errorf("payload.Fields[count] = %v, want 42", payload.Fields["count"])
	}

	if v, ok := payload.Fields["active"].(bool); !ok || !v {
		t.Errorf("payload.Fields[active] = %v, want true", payload.Fields["active"])
	}
}

func TestSearchFilterMustMatch(t *testing.T) {
	filter := NewSearchFilter().
		MustMatch("field1", "value1").
		MustMatch("field2", 42)

	if len(filter.Must) != 2 {
		t.Errorf("filter.Must length = %d, want 2", len(filter.Must))
	}

	if filter.Must[0].Field != "field1" || filter.Must[0].Value != "value1" {
		t.Errorf("filter.Must[0] = (%v, %v), want (field1, value1)", filter.Must[0].Field, filter.Must[0].Value)
	}
}
