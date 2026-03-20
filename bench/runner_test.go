package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestDataset(t *testing.T) (string, *DatasetLoader[string]) {
	t.Helper()
	dir := t.TempDir()
	manifest := DatasetManifest{
		Name:    "test-dataset",
		Version: "1.0",
		Samples: []ManifestSample{
			{ID: "s1", File: "s1.txt", Label: "positive"},
			{ID: "s2", File: "s2.txt", Label: "negative"},
			{ID: "s3", File: "s3.txt", Label: "positive"},
			{ID: "s4", File: "s4.txt", Label: "negative"},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"s1.txt", "s2.txt", "s3.txt", "s4.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loader := NewDatasetLoader[string](dir, func(s string) (string, error) { return s, nil })
	return dir, loader
}

// simpleMetric implements RunMetric for testing.
type simpleMetric struct {
	name string
}

func (m *simpleMetric) Name() string { return m.name }
func (m *simpleMetric) Compute(scored []ScoredSample[string]) MetricResult {
	correct := 0
	for _, s := range scored {
		if s.Sample.Label == s.Prediction.Label {
			correct++
		}
	}
	accuracy := float64(correct) / float64(len(scored))
	return MetricResult{
		Name:  m.name,
		Value: accuracy,
		Values: map[string]float64{
			"correct": float64(correct),
			"total":   float64(len(scored)),
		},
	}
}

func TestBenchRunnerBasic(t *testing.T) {
	t.Parallel()

	_, loader := setupTestDataset(t)
	runner := NewBenchRunner[string](
		WithMetrics[string](&simpleMetric{name: "accuracy"}),
	)
	runner.Register("perfect-model", EvaluatorFunc("perfect", func(ctx context.Context, input []byte) (Prediction[string], error) {
		// This evaluator always returns "positive" with score 0.9.
		return Prediction[string]{Label: "positive", Score: 0.9}, nil
	}))

	ctx := context.Background()
	result, err := runner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.ID == "" {
		t.Error("RunResult.ID is empty")
	}
	if result.Dataset.Name != "test-dataset" {
		t.Errorf("Dataset.Name = %q, want %q", result.Dataset.Name, "test-dataset")
	}
	if result.Dataset.SampleCount != 4 {
		t.Errorf("Dataset.SampleCount = %d, want 4", result.Dataset.SampleCount)
	}
	if len(result.Samples) != 4 {
		t.Errorf("len(Samples) = %d, want 4", len(result.Samples))
	}
	if len(result.Metrics) != 1 {
		t.Errorf("len(Metrics) = %d, want 1", len(result.Metrics))
	}
	// 2 out of 4 samples are "positive", so accuracy = 0.5
	if result.Metrics[0].Value != 0.5 {
		t.Errorf("accuracy = %f, want 0.5", result.Metrics[0].Value)
	}
}

func TestBenchRunnerMultipleBranches(t *testing.T) {
	t.Parallel()

	_, loader := setupTestDataset(t)
	runner := NewBenchRunner[string](
		WithMetrics[string](&simpleMetric{name: "accuracy"}),
	)

	// Branch 1: always predicts positive
	runner.Register("always-positive", EvaluatorFunc("pos", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "positive", Score: 0.9}, nil
	}))

	// Branch 2: always predicts negative
	runner.Register("always-negative", EvaluatorFunc("neg", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "negative", Score: 0.9}, nil
	}))

	ctx := context.Background()
	result, err := runner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(result.Branches) != 2 {
		t.Fatalf("len(Branches) = %d, want 2", len(result.Branches))
	}
	if _, ok := result.Branches["always-positive"]; !ok {
		t.Error("missing branch 'always-positive'")
	}
	if _, ok := result.Branches["always-negative"]; !ok {
		t.Error("missing branch 'always-negative'")
	}
}

func TestBenchRunnerWithConcurrency(t *testing.T) {
	t.Parallel()

	_, loader := setupTestDataset(t)
	runner := NewBenchRunner[string](
		WithMetrics[string](&simpleMetric{name: "accuracy"}),
		WithConcurrency[string](4),
	)
	runner.Register("concurrent", EvaluatorFunc("model", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "positive", Score: 0.8}, nil
	}))

	ctx := context.Background()
	result, err := runner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Dataset.SampleCount != 4 {
		t.Errorf("SampleCount = %d, want 4", result.Dataset.SampleCount)
	}
	if len(result.Samples) != 4 {
		t.Errorf("len(Samples) = %d, want 4", len(result.Samples))
	}
}

func TestBenchRunnerWithTag(t *testing.T) {
	t.Parallel()

	_, loader := setupTestDataset(t)
	runner := NewBenchRunner[string](
		WithTag[string]("v1-experiment"),
	)
	runner.Register("model", EvaluatorFunc("m", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "positive", Score: 0.5}, nil
	}))

	ctx := context.Background()
	result, err := runner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Tag != "v1-experiment" {
		t.Errorf("Tag = %q, want %q", result.Tag, "v1-experiment")
	}
}

func TestBenchRunnerNoBranches(t *testing.T) {
	t.Parallel()

	_, loader := setupTestDataset(t)
	runner := NewBenchRunner[string]()

	ctx := context.Background()
	_, err := runner.Run(ctx, loader)
	if err == nil {
		t.Fatal("expected error for no branches, got nil")
	}
}

func TestBenchRunnerEmptyDataset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := DatasetManifest{
		Name:    "empty",
		Version: "1.0",
		Samples: []ManifestSample{},
	}
	data, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)

	loader := NewDatasetLoader[string](dir, func(s string) (string, error) { return s, nil })
	runner := NewBenchRunner[string]()
	runner.Register("model", EvaluatorFunc("m", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{}, nil
	}))

	ctx := context.Background()
	_, err := runner.Run(ctx, loader)
	if err == nil {
		t.Fatal("expected error for empty dataset, got nil")
	}
}

func TestBenchRunnerWithStorage(t *testing.T) {
	t.Parallel()

	storageDir := t.TempDir()
	_, loader := setupTestDataset(t)

	storage := NewFileStorage(storageDir)
	runner := NewBenchRunner[string](
		WithStorage[string](storage),
	)
	runner.Register("model", EvaluatorFunc("m", func(ctx context.Context, input []byte) (Prediction[string], error) {
		return Prediction[string]{Label: "positive", Score: 0.5}, nil
	}))

	ctx := context.Background()
	result, err := runner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify result was stored.
	loaded, err := storage.Load(ctx, result.ID)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.ID != result.ID {
		t.Errorf("loaded ID = %q, want %q", loaded.ID, result.ID)
	}
}
