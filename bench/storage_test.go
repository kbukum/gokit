package bench

import (
	"context"
	"testing"
	"time"
)

func makeTestResult(id, tag, dataset string) *RunResult {
	return &RunResult{
		ID:        id,
		Schema:    SchemaVersion,
		Timestamp: time.Now(),
		Tag:       tag,
		Duration:  100 * time.Millisecond,
		Dataset: DatasetInfo{
			Name:              dataset,
			Version:           "1.0",
			SampleCount:       2,
			LabelDistribution: map[string]int{"positive": 1, "negative": 1},
		},
		Metrics: []MetricResult{
			{
				Name:  "classification",
				Value: 0.85,
				Values: map[string]float64{
					"precision": 0.9,
					"recall":    0.8,
					"f1":        0.85,
				},
			},
		},
		Branches: map[string]BranchResult{
			"main": {Name: "main", Tier: 0, Metrics: map[string]float64{"f1": 0.85}},
		},
		Samples: []SampleResult{
			{ID: "s1", Label: "positive", Predicted: "positive", Score: 0.95, Correct: true},
			{ID: "s2", Label: "negative", Predicted: "positive", Score: 0.6, Correct: false},
		},
	}
}

func TestFileStorageSaveAndLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	result := makeTestResult("run-001", "test", "my-dataset")
	savedID, err := storage.Save(ctx, result)
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if savedID != "run-001" {
		t.Errorf("savedID = %q, want %q", savedID, "run-001")
	}

	loaded, err := storage.Load(ctx, "run-001")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.ID != result.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, result.ID)
	}
	if loaded.Dataset.Name != result.Dataset.Name {
		t.Errorf("Dataset.Name = %q, want %q", loaded.Dataset.Name, result.Dataset.Name)
	}
	if len(loaded.Metrics) != len(result.Metrics) {
		t.Errorf("len(Metrics) = %d, want %d", len(loaded.Metrics), len(result.Metrics))
	}
	if len(loaded.Samples) != len(result.Samples) {
		t.Errorf("len(Samples) = %d, want %d", len(loaded.Samples), len(result.Samples))
	}
}

func TestFileStorageLoadNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	_, err := storage.Load(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent run, got nil")
	}
}

func TestFileStorageLatest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	r1 := makeTestResult("run-old", "v1", "ds")
	r1.Timestamp = time.Now().Add(-1 * time.Hour)
	r2 := makeTestResult("run-new", "v2", "ds")
	r2.Timestamp = time.Now()

	storage.Save(ctx, r1)
	storage.Save(ctx, r2)

	latest, err := storage.Latest(ctx)
	if err != nil {
		t.Fatalf("Latest() error: %v", err)
	}
	if latest.ID != "run-new" {
		t.Errorf("Latest().ID = %q, want %q", latest.ID, "run-new")
	}
}

func TestFileStorageList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	for i, id := range []string{"run-a", "run-b", "run-c"} {
		r := makeTestResult(id, "tag", "ds")
		r.Timestamp = time.Now().Add(time.Duration(i) * time.Minute)
		storage.Save(ctx, r)
	}

	summaries, err := storage.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("len(summaries) = %d, want 3", len(summaries))
	}
	// Should be sorted by timestamp descending.
	if summaries[0].ID != "run-c" {
		t.Errorf("summaries[0].ID = %q, want %q", summaries[0].ID, "run-c")
	}
}

func TestFileStorageListWithLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	for i, id := range []string{"run-1", "run-2", "run-3"} {
		r := makeTestResult(id, "", "ds")
		r.Timestamp = time.Now().Add(time.Duration(i) * time.Minute)
		storage.Save(ctx, r)
	}

	summaries, err := storage.List(ctx, WithLimit(2))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("len(summaries) = %d, want 2", len(summaries))
	}
}

func TestFileStorageListWithTagFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	storage.Save(ctx, makeTestResult("run-a", "v1", "ds"))
	storage.Save(ctx, makeTestResult("run-b", "v2", "ds"))
	storage.Save(ctx, makeTestResult("run-c", "v1", "ds"))

	summaries, err := storage.List(ctx, WithTagFilter("v1"))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("len(summaries) = %d, want 2", len(summaries))
	}
}

func TestFileStorageListWithDatasetFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	storage.Save(ctx, makeTestResult("run-a", "", "dataset-a"))
	storage.Save(ctx, makeTestResult("run-b", "", "dataset-b"))

	summaries, err := storage.List(ctx, WithDatasetFilter("dataset-a"))
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("len(summaries) = %d, want 1", len(summaries))
	}
}

func TestFileStorageEmptyDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	summaries, err := storage.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("len(summaries) = %d, want 0", len(summaries))
	}
}

func TestFileStorageLatestEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := NewFileStorage(dir)
	ctx := context.Background()

	_, err := storage.Latest(ctx)
	if err == nil {
		t.Fatal("expected error for empty storage, got nil")
	}
}

func TestResolveListOptions(t *testing.T) {
	t.Parallel()

	params := ResolveListOptions(WithLimit(50), WithTagFilter("v1"), WithDatasetFilter("ds"))
	if params.Limit != 50 {
		t.Errorf("Limit = %d, want 50", params.Limit)
	}
	if params.Tag != "v1" {
		t.Errorf("Tag = %q, want %q", params.Tag, "v1")
	}
	if params.Dataset != "ds" {
		t.Errorf("Dataset = %q, want %q", params.Dataset, "ds")
	}
}

func TestResolveListOptionsDefaults(t *testing.T) {
	t.Parallel()

	params := ResolveListOptions()
	if params.Limit != 100 {
		t.Errorf("default Limit = %d, want 100", params.Limit)
	}
}
