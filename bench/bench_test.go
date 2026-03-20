package bench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/bench/report"
	"github.com/kbukum/gokit/bench/viz"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// createDataset writes a manifest.json and 10 sample files to dir.
// 5 samples are labelled "ai_generated", 5 are "human_created".
func createDataset(t *testing.T, dir string) {
	t.Helper()

	samples := []bench.ManifestSample{
		{ID: "ai-1", File: "ai_1.txt", Label: "ai_generated"},
		{ID: "ai-2", File: "ai_2.txt", Label: "ai_generated"},
		{ID: "ai-3", File: "ai_3.txt", Label: "ai_generated"},
		{ID: "ai-4", File: "ai_4.txt", Label: "ai_generated"},
		{ID: "ai-5", File: "ai_5.txt", Label: "ai_generated"},
		{ID: "hu-1", File: "hu_1.txt", Label: "human_created"},
		{ID: "hu-2", File: "hu_2.txt", Label: "human_created"},
		{ID: "hu-3", File: "hu_3.txt", Label: "human_created"},
		{ID: "hu-4", File: "hu_4.txt", Label: "human_created"},
		{ID: "hu-5", File: "hu_5.txt", Label: "human_created"},
	}

	manifest := bench.DatasetManifest{
		Name:    "e2e-dataset",
		Version: "1.0",
		Samples: samples,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// AI samples get short text; human samples get longer text with keywords.
	aiTexts := []string{
		"short ai text",
		"tiny ai",
		"brief",
		"small sample",
		"ai mini",
	}
	humanTexts := []string{
		"this is a longer human written text with natural language patterns and ideas",
		"another lengthy human passage describing complex thoughts and human concepts",
		"elaborate human paragraph about deep topics with many words inside it here",
		"extensive human authored document containing human style and human vocabulary",
		"substantial human narrative with rich expressions and human creativity shown",
	}

	for i, txt := range aiTexts {
		name := samples[i].File
		if err := os.WriteFile(filepath.Join(dir, name), []byte(txt), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i, txt := range humanTexts {
		name := samples[5+i].File
		if err := os.WriteFile(filepath.Join(dir, name), []byte(txt), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stringMapper is a LabelMapper that passes labels through as-is.
func stringMapper(s string) (string, error) { return s, nil }

// branchAEvaluator assigns high scores to short texts (< 30 bytes ⇒ ai_generated).
func branchAEvaluator() bench.Evaluator[string] {
	return bench.EvaluatorFunc("branch-a-model", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		score := 0.3
		label := "human_created"
		if len(input) < 30 {
			score = 0.9
			label = "ai_generated"
		}
		return bench.Prediction[string]{
			Label: label,
			Score: score,
			Scores: map[string]float64{
				"ai_generated":  score,
				"human_created": 1 - score,
			},
		}, nil
	})
}

// branchBEvaluator assigns high scores when "human" keyword is absent.
func branchBEvaluator() bench.Evaluator[string] {
	return bench.EvaluatorFunc("branch-b-model", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		hasHuman := strings.Contains(strings.ToLower(string(input)), "human")
		score := 0.8
		label := "ai_generated"
		if hasHuman {
			score = 0.2
			label = "human_created"
		}
		return bench.Prediction[string]{
			Label: label,
			Score: score,
			Scores: map[string]float64{
				"ai_generated":  score,
				"human_created": 1 - score,
			},
		}, nil
	})
}

// ── end-to-end test ──────────────────────────────────────────────────────────

func TestEndToEnd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dataDir := t.TempDir()
	storageDir := t.TempDir()

	// ── 1. Setup dataset ──
	createDataset(t, dataDir)

	// ── 2. Create dataset loader ──
	loader := bench.NewDatasetLoader[string](dataDir, stringMapper)

	// Sanity-check the manifest loads.
	manifest, err := loader.Manifest()
	if err != nil {
		t.Fatalf("Manifest() error: %v", err)
	}
	if len(manifest.Samples) != 10 {
		t.Fatalf("manifest sample count = %d, want 10", len(manifest.Samples))
	}

	// ── 3 & 4. Evaluators with middleware ──
	evalA := bench.WithTiming[string](branchAEvaluator())
	evalB := branchBEvaluator()

	// ── 5. Create runner (run 1) ──
	// Use BinaryClassification and ExactMatch for storage-safe runs
	// (AUCROC/BrierScore can produce +Inf which is not JSON-serializable).
	storage := bench.NewFileStorage(storageDir)
	runner1 := bench.NewBenchRunner[string](
		bench.WithMetrics[string](metric.AsRunMetrics[string](
			metric.BinaryClassification[string]("ai_generated"),
			metric.ExactMatch[string](),
		)...),
		bench.WithStorage[string](storage),
		bench.WithConcurrency[string](2),
		bench.WithTag[string]("e2e-test-v1"),
	)
	runner1.Register("branch-a", evalA)
	runner1.Register("branch-b", evalB)

	// ── 6. Run first benchmark ──
	result1, err := runner1.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() v1 error: %v", err)
	}

	// ── 7. Verify RunResult ──
	t.Run("verify_run_result_v1", func(t *testing.T) {
		if result1.ID == "" {
			t.Error("RunResult.ID is empty")
		}
		if result1.Tag != "e2e-test-v1" {
			t.Errorf("Tag = %q, want %q", result1.Tag, "e2e-test-v1")
		}
		if result1.Dataset.SampleCount != 10 {
			t.Errorf("SampleCount = %d, want 10", result1.Dataset.SampleCount)
		}
		if result1.Dataset.Name != "e2e-dataset" {
			t.Errorf("Dataset.Name = %q, want %q", result1.Dataset.Name, "e2e-dataset")
		}
		if len(result1.Samples) != 10 {
			t.Errorf("len(Samples) = %d, want 10", len(result1.Samples))
		}
		if len(result1.Metrics) == 0 {
			t.Fatal("no metrics in result")
		}

		// Check both branches present.
		if _, ok := result1.Branches["branch-a"]; !ok {
			t.Error("missing branch 'branch-a'")
		}
		if _, ok := result1.Branches["branch-b"]; !ok {
			t.Error("missing branch 'branch-b'")
		}

		// Every sample should have a prediction label.
		for _, s := range result1.Samples {
			if s.Predicted == "" {
				t.Errorf("sample %s has empty Predicted label", s.ID)
			}
		}
	})

	// ── 7b. Verify middleware captured timings ──
	t.Run("verify_timing_middleware", func(t *testing.T) {
		timings := evalA.Timings()
		if len(timings) == 0 {
			t.Error("TimingMiddleware recorded no timings")
		}
	})

	// ── 8. Run second benchmark (slightly different evaluators) ──
	// branch-a now inverts its logic to produce different metrics.
	invertedEvalA := bench.EvaluatorFunc("branch-a-inverted", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		score := 0.8
		label := "ai_generated"
		if len(input) < 30 {
			score = 0.2
			label = "human_created"
		}
		return bench.Prediction[string]{
			Label: label,
			Score: score,
			Scores: map[string]float64{
				"ai_generated":  score,
				"human_created": 1 - score,
			},
		}, nil
	})

	runner2 := bench.NewBenchRunner[string](
		bench.WithMetrics[string](metric.AsRunMetrics[string](
			metric.BinaryClassification[string]("ai_generated"),
			metric.ExactMatch[string](),
		)...),
		bench.WithStorage[string](storage),
		bench.WithConcurrency[string](2),
		bench.WithTag[string]("e2e-test-v2"),
	)
	runner2.Register("branch-a", invertedEvalA)
	runner2.Register("branch-b", evalB)

	result2, err := runner2.Run(ctx, loader)
	if err != nil {
		t.Fatalf("Run() v2 error: %v", err)
	}

	// ── 9. Compare runs ──
	t.Run("compare_runs", func(t *testing.T) {
		comparator := bench.NewRunComparator()
		diff := comparator.Compare(result1, result2)

		if diff.BaseID != result1.ID {
			t.Errorf("diff.BaseID = %q, want %q", diff.BaseID, result1.ID)
		}
		if diff.TargetID != result2.ID {
			t.Errorf("diff.TargetID = %q, want %q", diff.TargetID, result2.ID)
		}
		if len(diff.Changes) == 0 {
			t.Error("expected metric changes between runs, got none")
		}

		summary := diff.Summary()
		if summary == "" {
			t.Error("diff.Summary() returned empty string")
		}
		t.Logf("Diff summary:\n%s", summary)
	})

	// ── 10. Storage round-trip ──
	t.Run("storage_round_trip", func(t *testing.T) {
		loaded, err := storage.Load(ctx, result1.ID)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if loaded.ID != result1.ID {
			t.Errorf("loaded.ID = %q, want %q", loaded.ID, result1.ID)
		}
		if loaded.Tag != result1.Tag {
			t.Errorf("loaded.Tag = %q, want %q", loaded.Tag, result1.Tag)
		}
		if loaded.Dataset.SampleCount != result1.Dataset.SampleCount {
			t.Errorf("loaded.SampleCount = %d, want %d",
				loaded.Dataset.SampleCount, result1.Dataset.SampleCount)
		}
		if len(loaded.Metrics) != len(result1.Metrics) {
			t.Errorf("loaded metric count = %d, want %d",
				len(loaded.Metrics), len(result1.Metrics))
		}

		// Verify Latest() returns the second run.
		latest, err := storage.Latest(ctx)
		if err != nil {
			t.Fatalf("Latest() error: %v", err)
		}
		if latest.ID != result2.ID {
			t.Errorf("Latest().ID = %q, want %q", latest.ID, result2.ID)
		}

		// Verify List() returns both runs.
		summaries, err := storage.List(ctx)
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}
		if len(summaries) < 2 {
			t.Errorf("List() returned %d summaries, want >= 2", len(summaries))
		}
	})

	// ── 11. Generate all reports ──
	reporters := map[string]report.Reporter{
		"json":     report.JSON(),
		"markdown": report.Markdown(),
		"table":    report.Table(),
		"csv":      report.CSV(),
		"junit":    report.JUnit(report.WithTargets(map[string]float64{"binary_classification": 0.5})),
		"vegalite": report.VegaLite(),
		"html":     report.HTML(),
	}

	for name, r := range reporters {
		t.Run("report_"+name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := r.Generate(&buf, result1); err != nil {
				t.Fatalf("%s Generate() error: %v", name, err)
			}
			output := buf.String()
			if output == "" {
				t.Fatalf("%s produced empty output", name)
			}

			switch name {
			case "json":
				var js map[string]any
				if err := json.Unmarshal([]byte(output), &js); err != nil {
					t.Errorf("JSON report is not valid JSON: %v", err)
				}
			case "junit":
				if !strings.Contains(output, "<testsuite") {
					t.Error("JUnit report missing <testsuite> tag")
				}
			case "html":
				if !strings.Contains(strings.ToLower(output), "<html") {
					t.Error("HTML report missing <html> tag")
				}
			case "csv":
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) < 2 {
					t.Error("CSV report has fewer than 2 lines (header + data)")
				}
				for _, line := range lines {
					if !strings.Contains(line, ",") {
						t.Errorf("CSV line missing commas: %q", line)
					}
				}
			case "markdown":
				if !strings.Contains(output, "|") {
					t.Error("Markdown report missing table pipe characters")
				}
			case "vegalite":
				var vl map[string]any
				if err := json.Unmarshal([]byte(output), &vl); err != nil {
					t.Errorf("VegaLite report is not valid JSON: %v", err)
				}
			}
		})
	}

	// ── 12. Generate SVG visualizations ──
	t.Run("viz_render_all", func(t *testing.T) {
		svgs := viz.RenderAll(result1)
		if len(svgs) == 0 {
			t.Log("viz.RenderAll returned no SVGs (may require curve data)")
			return
		}
		for key, svg := range svgs {
			if svg == "" {
				t.Errorf("SVG for %q is empty", key)
			}
			if !strings.Contains(svg, "<svg") {
				t.Errorf("SVG for %q missing <svg tag", key)
			}
		}
	})

	// ── 13. Probability metrics (AUCROC, BrierScore) without storage ──
	t.Run("probability_metrics", func(t *testing.T) {
		probRunner := bench.NewBenchRunner[string](
			bench.WithMetrics[string](metric.AsRunMetrics[string](
				metric.AUCROC[string]("ai_generated"),
				metric.BrierScore[string]("ai_generated"),
			)...),
		)
		probRunner.Register("branch-a", branchAEvaluator())
		probResult, err := probRunner.Run(ctx, loader)
		if err != nil {
			t.Fatalf("probability Run() error: %v", err)
		}
		if len(probResult.Metrics) == 0 {
			t.Fatal("no probability metrics in result")
		}
		for _, m := range probResult.Metrics {
			t.Logf("probability metric %s = %f", m.Name, m.Value)
		}
	})
}

// ── regression detection test ────────────────────────────────────────────────

func TestEndToEndWithRegression(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dataDir := t.TempDir()
	createDataset(t, dataDir)
	loader := bench.NewDatasetLoader[string](dataDir, stringMapper)

	// Baseline: good evaluator (correctly classifies most samples).
	goodEval := bench.EvaluatorFunc("good-model", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		hasHuman := strings.Contains(strings.ToLower(string(input)), "human")
		score := 0.85
		label := "ai_generated"
		if hasHuman {
			score = 0.15
			label = "human_created"
		}
		return bench.Prediction[string]{
			Label: label,
			Score: score,
			Scores: map[string]float64{
				"ai_generated":  score,
				"human_created": 1 - score,
			},
		}, nil
	})

	// Target: bad evaluator (inverts predictions).
	badEval := bench.EvaluatorFunc("bad-model", func(_ context.Context, input []byte) (bench.Prediction[string], error) {
		hasHuman := strings.Contains(strings.ToLower(string(input)), "human")
		score := 0.15
		label := "human_created"
		if hasHuman {
			score = 0.85
			label = "ai_generated"
		}
		return bench.Prediction[string]{
			Label: label,
			Score: score,
			Scores: map[string]float64{
				"ai_generated":  score,
				"human_created": 1 - score,
			},
		}, nil
	})

	metrics := metric.AsRunMetrics[string](
		metric.BinaryClassification[string]("ai_generated"),
		metric.ExactMatch[string](),
	)

	// Run baseline.
	baseRunner := bench.NewBenchRunner[string](
		bench.WithMetrics[string](metrics...),
		bench.WithTag[string]("baseline"),
	)
	baseRunner.Register("model", goodEval)

	baseResult, err := baseRunner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("baseline Run() error: %v", err)
	}

	// Run target with worse model.
	targetRunner := bench.NewBenchRunner[string](
		bench.WithMetrics[string](metrics...),
		bench.WithTag[string]("regression-target"),
	)
	targetRunner.Register("model", badEval)

	targetResult, err := targetRunner.Run(ctx, loader)
	if err != nil {
		t.Fatalf("target Run() error: %v", err)
	}

	// Compare and detect regression.
	comparator := bench.NewRunComparator()
	diff := comparator.Compare(baseResult, targetResult)

	t.Logf("Regression diff summary:\n%s", diff.Summary())

	if !diff.HasRegression() {
		t.Error("expected HasRegression() = true for degraded model, got false")
	}
	if len(diff.Regressed) == 0 {
		t.Error("expected Regressed list to be non-empty")
	}
}
