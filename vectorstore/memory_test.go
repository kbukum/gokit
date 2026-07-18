package vectorstore

import (
	"context"
	"errors"
	"math"
	"strings"
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
	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{1.0, 0.0, 0.0}, Payload: payload1})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	payload2 := NewPointPayload().WithField("name", "doc2")
	err = store.Upsert(ctx, "test", Point{ID: "2", Vector: []float32{0.0, 1.0, 0.0}, Payload: payload2})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{1.0, 0.0, 0.0}, Limit: 10, Filter: nil})
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
	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{1.0, 0.0}, Payload: payload1})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	payload2 := NewPointPayload().WithField("v", "new")
	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{0.0, 1.0}, Payload: payload2})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{0.0, 1.0}, Limit: 10, Filter: nil})
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

	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{1.0, 0.0}, Payload: NewPointPayload().WithField("type", "a")})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	err = store.Upsert(ctx, "test", Point{ID: "2", Vector: []float32{1.0, 0.0}, Payload: NewPointPayload().WithField("type", "b")})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	filter := NewSearchFilter().MustMatch("type", "a")
	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{1.0, 0.0}, Limit: 10, Filter: filter})
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

	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{1.0, 0.0}, Payload: NewPointPayload()})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	err = store.Delete(ctx, "test", "1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{1.0, 0.0}, Limit: 10, Filter: nil})
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

	err = store.Upsert(ctx, "test", Point{ID: "1", Vector: []float32{1.0, 0.0}, Payload: NewPointPayload()})
	if err == nil {
		t.Error("Upsert() expected error for dimension mismatch")
	}
}

func TestInMemoryStoreUpsertMissingCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.Upsert(ctx, "nonexistent", Point{ID: "1", Vector: []float32{1.0}, Payload: NewPointPayload()})
	if err == nil {
		t.Error("Upsert() expected error for missing collection")
	}
}

func TestInMemoryStoreSearchMissingCollection(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	_, err := store.Search(ctx, "nonexistent", SearchQuery{Vector: []float32{1.0}, Limit: 10, Filter: nil})
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
		err = store.Upsert(ctx, "test", Point{ID: string(rune('1' + i)), Vector: []float32{float32(i)}, Payload: payload})
		if err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	// Search with limit
	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{4.0}, Limit: 2, Filter: nil})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}
}

func TestInMemoryStoreSearchDotMetric(t *testing.T) {
	store, err := NewInMemoryStoreWithConfig(Config{Metric: MetricDot})
	if err != nil {
		t.Fatalf("NewInMemoryStoreWithConfig() error = %v", err)
	}
	ctx := context.Background()
	err = store.EnsureCollection(ctx, "test", 2)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	err = store.Upsert(ctx, "test", Point{ID: "strong", Vector: []float32{2, 0}, Payload: NewPointPayload()})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	err = store.Upsert(ctx, "test", Point{ID: "weak", Vector: []float32{0, 1}, Payload: NewPointPayload()})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{1, 1}, Limit: 2, Filter: nil})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results[0].ID != "strong" || results[0].Score != 2 {
		t.Fatalf("dot ranking = (%s, %v), want (strong, 2)", results[0].ID, results[0].Score)
	}
}

func TestInMemoryStoreSearchL2Metric(t *testing.T) {
	store, err := NewInMemoryStoreWithConfig(Config{Metric: MetricL2})
	if err != nil {
		t.Fatalf("NewInMemoryStoreWithConfig() error = %v", err)
	}
	ctx := context.Background()
	err = store.EnsureCollection(ctx, "test", 1)
	if err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	err = store.Upsert(ctx, "test", Point{ID: "near", Vector: []float32{1}, Payload: NewPointPayload()})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	err = store.Upsert(ctx, "test", Point{ID: "far", Vector: []float32{3}, Payload: NewPointPayload()})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	results, err := store.Search(ctx, "test", SearchQuery{Vector: []float32{0}, Limit: 2, Filter: nil})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if results[0].ID != "near" || results[0].Score != -1 {
		t.Fatalf("l2 ranking = (%s, %v), want (near, -1)", results[0].ID, results[0].Score)
	}
}

func TestInMemoryStoreUnknownMetricReturnsMetricError(t *testing.T) {
	_, err := NewInMemoryStoreWithConfig(Config{Metric: "unknown"})
	var metricErr *MetricError
	if !errors.As(err, &metricErr) {
		t.Fatalf("NewInMemoryStoreWithConfig() error = %T %[1]v, want MetricError", err)
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

func TestInMemoryStoreSearchWrongDimensions(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "c", 3); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if _, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 2}, Limit: 5, Filter: nil}); err == nil {
		t.Fatal("expected dimension mismatch error")
	}
}

func TestInMemoryStoreDeleteKeepsOtherPoints(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if err := store.Upsert(ctx, "c", Point{ID: "a", Vector: []float32{1, 0}, Payload: nil}); err != nil {
		t.Fatalf("Upsert a: %v", err)
	}
	if err := store.Upsert(ctx, "c", Point{ID: "b", Vector: []float32{0, 1}, Payload: nil}); err != nil {
		t.Fatalf("Upsert b: %v", err)
	}
	if err := store.Delete(ctx, "c", "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	results, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{0, 1}, Limit: 5, Filter: nil})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "b" {
		t.Fatalf("expected only point b to remain, got %+v", results)
	}
}

func TestInMemoryStoreFilterSkipsNilPayload(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if err := store.Upsert(ctx, "c", Point{ID: "a", Vector: []float32{1, 0}, Payload: nil}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	filter := NewSearchFilter().MustMatch("kind", "doc")
	results, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, Limit: 5, Filter: filter})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("nil-payload point must not match filter, got %+v", results)
	}
}

func TestInMemoryStoreFilterMatchesStructuredValue(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	payload := NewPointPayload().WithField("tags", []string{"x", "y"})
	if err := store.Upsert(ctx, "c", Point{ID: "a", Vector: []float32{1, 0}, Payload: payload}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	match := NewSearchFilter().MustMatch("tags", []string{"x", "y"})
	results, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, Limit: 5, Filter: match})
	if err != nil {
		t.Fatalf("Search match: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected structured value to match via deep compare, got %+v", results)
	}
	miss := NewSearchFilter().MustMatch("tags", []string{"z"})
	results, err = store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, Limit: 5, Filter: miss})
	if err != nil {
		t.Fatalf("Search miss: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected structured value mismatch, got %+v", results)
	}
}

func TestMetricErrorMessage(t *testing.T) {
	t.Parallel()

	err := &MetricError{Metric: "hamming"}
	msg := err.Error()
	if !strings.Contains(msg, "hamming") || !strings.Contains(msg, MetricCosine) {
		t.Fatalf("unexpected metric error message: %q", msg)
	}
}

func TestInMemoryStoreFilterMatchesNilValue(t *testing.T) {
	t.Parallel()

	store := NewInMemoryStore()
	ctx := context.Background()
	if err := store.EnsureCollection(ctx, "c", 2); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if err := store.Upsert(ctx, "c", Point{ID: "a", Vector: []float32{1, 0}, Payload: NewPointPayload().WithField("owner", nil)}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	filter := NewSearchFilter().MustMatch("owner", nil)
	results, err := store.Search(ctx, "c", SearchQuery{Vector: []float32{1, 0}, Limit: 5, Filter: filter})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("nil field value should match nil filter value, got %+v", results)
	}
}

func TestConfigApplyDefaults(t *testing.T) {
	t.Parallel()

	var cfg Config
	cfg.ApplyDefaults()
	if cfg.Provider != DefaultProvider {
		t.Errorf("Provider default = %q, want %q", cfg.Provider, DefaultProvider)
	}
	if cfg.Metric != DefaultMetric {
		t.Errorf("Metric default = %q, want %q", cfg.Metric, DefaultMetric)
	}

	custom := Config{Provider: "qdrant", Metric: MetricL2}
	custom.ApplyDefaults()
	if custom.Provider != "qdrant" || custom.Metric != MetricL2 {
		t.Errorf("ApplyDefaults overrode explicit values: %+v", custom)
	}
}
