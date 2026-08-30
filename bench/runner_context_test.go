package bench_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/embedding/inmem"
	"github.com/kbukum/gokit/util"
)

func writeContextDataset(t *testing.T) *bench.DatasetLoader[string] {
	t.Helper()
	dir := t.TempDir()
	manifest := bench.DatasetManifest{
		Name:    "ctx-dataset",
		Version: "1.0",
		Samples: []bench.ManifestSample{
			{ID: "s1", File: "s1.txt", Label: "positive"},
			{ID: "s2", File: "s2.txt", Label: "negative"},
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := util.WriteFile(filepath.Join(dir, "manifest.json"), data); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"s1.txt", "s2.txt"} {
		if err := util.WriteFile(filepath.Join(dir, name), []byte("content-"+name)); err != nil {
			t.Fatal(err)
		}
	}
	return bench.NewDatasetLoader(dir, func(s string) (string, error) { return s, nil })
}

// The runner computes pure metrics first, then context metrics, merging results
// in registration order (pure first, then context).
func TestRunnerRunsContextMetricsAfterSyncMetrics(t *testing.T) {
	t.Parallel()

	loader := writeContextDataset(t)

	semantic, err := metric.SemanticSimilarity[string](inmem.New(8), ai.Model{Name: "test-embed"})
	if err != nil {
		t.Fatalf("SemanticSimilarity: %v", err)
	}

	runner := bench.NewBenchRunner(
		bench.WithMetrics(metric.AsRunMetric[string](mustBinaryClassification[string](t, "positive"))),
		bench.WithContextMetrics(metric.AsRunContextMetric[string](semantic)),
	)
	runner.Register("model", bench.EvaluatorFunc("m", func(_ context.Context, _ []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "positive", Score: 0.9}, nil
	}))

	result, err := runner.Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.Metrics) != 2 {
		t.Fatalf("len(Metrics) = %d, want 2", len(result.Metrics))
	}
	// Sync metric first in registration order, context metric after.
	if result.Metrics[len(result.Metrics)-1].Name != "semantic_similarity[test-embed:t0.8]" {
		t.Errorf("last metric = %q, want semantic_similarity[test-embed:t0.8] (context metric runs after sync)", result.Metrics[len(result.Metrics)-1].Name)
	}
	// Provenance metric names include both phases.
	found := false
	for _, n := range result.Provenance.Metrics {
		if n == "semantic_similarity[test-embed:t0.8]" {
			found = true
		}
	}
	if !found {
		t.Errorf("provenance metrics %v missing the context metric", result.Provenance.Metrics)
	}
}

// A context metric returning an error fails the whole run.
func TestRunnerFailsWhenContextMetricErrors(t *testing.T) {
	t.Parallel()

	loader := writeContextDataset(t)
	runner := bench.NewBenchRunner(
		bench.WithContextMetrics[string](&failingContextMetric{name: "boom"}),
	)
	runner.Register("model", bench.EvaluatorFunc("m", func(_ context.Context, _ []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: "positive"}, nil
	}))

	if _, err := runner.Run(context.Background(), loader); err == nil {
		t.Fatal("expected run to fail when a context metric errors, got nil")
	}
}

type failingContextMetric struct{ name string }

func (m *failingContextMetric) Name() string { return m.name }

func (m *failingContextMetric) Compute(context.Context, []bench.ScoredSample[string]) (bench.MetricResult, error) {
	return bench.MetricResult{}, context.DeadlineExceeded
}
