package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
)

// makeTestResult builds a realistic RunResult with metrics, branches,
// samples, and curve data suitable for exercising every reporter.
func makeTestResult() *bench.RunResult {
	return &bench.RunResult{
		ID:        "run-abc-123",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		Tag:       "nightly",
		Duration:  5 * time.Second,
		Dataset: bench.DatasetInfo{
			Name:              "eval-dataset",
			Version:           "2.1",
			SampleCount:       5,
			LabelDistribution: map[string]int{"positive": 3, "negative": 2},
		},
		Metrics: []bench.MetricResult{
			{
				Name:  "f1",
				Value: 0.88,
				Values: map[string]float64{
					"positive": 0.90,
					"negative": 0.85,
				},
				Detail: &bench.ConfusionMatrixDetail{
					Labels: []string{"positive", "negative"},
					Matrix: [][]int{
						{40, 5},
						{3, 22},
					},
					Orientation: "row=actual, col=predicted",
				},
			},
			{
				Name:  "auc",
				Value: 0.92,
			},
		},
		Branches: map[string]bench.BranchResult{
			"keyword": {
				Name:             "keyword",
				Tier:             1,
				Metrics:          map[string]float64{"f1": 0.80, "precision": 0.85},
				AvgScorePositive: 0.88,
				AvgScoreNegative: 0.25,
				Duration:         2 * time.Second,
				Errors:           0,
			},
			"semantic": {
				Name:             "semantic",
				Tier:             2,
				Metrics:          map[string]float64{"f1": 0.92, "precision": 0.90},
				AvgScorePositive: 0.95,
				AvgScoreNegative: 0.15,
				Duration:         3 * time.Second,
				Errors:           1,
			},
		},
		Samples: []bench.SampleResult{
			{ID: "s1", Label: "positive", Predicted: "positive", Score: 0.95, Correct: true, Duration: 100 * time.Millisecond},
			{ID: "s2", Label: "positive", Predicted: "positive", Score: 0.82, Correct: true, Duration: 120 * time.Millisecond},
			{ID: "s3", Label: "positive", Predicted: "negative", Score: 0.45, Correct: false, Duration: 110 * time.Millisecond},
			{ID: "s4", Label: "negative", Predicted: "negative", Score: 0.88, Correct: true, Duration: 90 * time.Millisecond},
			{ID: "s5", Label: "negative", Predicted: "positive", Score: 0.55, Correct: false, Duration: 95 * time.Millisecond, Error: "low confidence"},
		},
		Curves: map[string]any{
			"roc": bench.ROCCurve{
				FPR:        []float64{0.0, 0.1, 0.3, 0.5, 1.0},
				TPR:        []float64{0.0, 0.5, 0.7, 0.9, 1.0},
				Thresholds: []float64{1.0, 0.8, 0.5, 0.3, 0.0},
				AUC:        0.92,
			},
			"calibration": bench.CalibrationCurve{
				PredictedProbability: []float64{0.1, 0.3, 0.5, 0.7, 0.9},
				ActualFrequency:      []float64{0.12, 0.28, 0.52, 0.68, 0.91},
				BinCount:             []int{10, 12, 15, 11, 8},
			},
			"score_distribution": []bench.ScoreDistribution{
				{
					Label:  "positive",
					Bins:   []float64{0.0, 0.2, 0.4, 0.6, 0.8, 1.0},
					Counts: []int{1, 2, 5, 8, 4},
				},
				{
					Label:  "negative",
					Bins:   []float64{0.0, 0.2, 0.4, 0.6, 0.8, 1.0},
					Counts: []int{6, 4, 2, 1, 0},
				},
			},
			"threshold_sweep": []bench.ThresholdPoint{
				{Threshold: 0.3, Precision: 0.70, Recall: 0.95, F1: 0.81, Accuracy: 0.75},
				{Threshold: 0.5, Precision: 0.85, Recall: 0.80, F1: 0.82, Accuracy: 0.84},
				{Threshold: 0.7, Precision: 0.92, Recall: 0.60, F1: 0.73, Accuracy: 0.80},
			},
		},
	}
}

// makeMinimalResult returns a RunResult with only metrics — no branches, samples, or curves.
func makeMinimalResult() *bench.RunResult {
	return &bench.RunResult{
		ID:        "minimal-run",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Duration:  time.Second,
		Dataset: bench.DatasetInfo{
			Name:        "minimal",
			Version:     "1.0",
			SampleCount: 0,
		},
		Metrics: []bench.MetricResult{
			{Name: "accuracy", Value: 0.75},
		},
	}
}

// makeEmptyResult returns a completely empty RunResult.
func makeEmptyResult() *bench.RunResult {
	return &bench.RunResult{
		ID:        "empty-run",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Now(),
	}
}

// --- JSON Reporter ---

func TestJSONReporter(t *testing.T) {
	t.Parallel()
	r := JSON()
	result := makeTestResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatal("empty output")
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check top-level fields
	if schema, ok := parsed["$schema"].(string); !ok || schema != bench.SchemaURL {
		t.Errorf("$schema = %v, want %q", parsed["$schema"], bench.SchemaURL)
	}
	if version, ok := parsed["version"].(string); !ok || version != bench.SchemaVersion {
		t.Errorf("version = %v, want %q", parsed["version"], bench.SchemaVersion)
	}

	// Run section
	run, ok := parsed["run"].(map[string]any)
	if !ok {
		t.Fatal("missing 'run' section")
	}
	if run["id"] != "run-abc-123" {
		t.Errorf("run.id = %v, want run-abc-123", run["id"])
	}
	if run["tag"] != "nightly" {
		t.Errorf("run.tag = %v, want nightly", run["tag"])
	}

	// Metrics
	metrics, ok := parsed["metrics"].([]any)
	if !ok || len(metrics) != 2 {
		t.Errorf("expected 2 metrics, got %v", metrics)
	}

	// Branches
	branches, ok := parsed["branches"].([]any)
	if !ok || len(branches) != 2 {
		t.Errorf("expected 2 branches, got %v", branches)
	}

	// Samples
	samples, ok := parsed["samples"].([]any)
	if !ok || len(samples) != 5 {
		t.Errorf("expected 5 samples, got %v", samples)
	}

	// Curves
	if _, ok := parsed["curves"]; !ok {
		t.Error("missing 'curves' section")
	}
}

// --- Markdown Reporter ---

func TestMarkdownReporter(t *testing.T) {
	t.Parallel()
	r := Markdown()
	result := makeTestResult()

	if r.Name() != "markdown" {
		t.Errorf("Name() = %q, want markdown", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	checks := []struct {
		name, needle string
	}{
		{"report header", "# Bench Run Report"},
		{"metrics heading", "## Metrics"},
		{"confusion heading", "## Confusion Matrix"},
		{"branches heading", "## Branches"},
		{"samples heading", "## Samples"},
		{"table separator", "|---|"},
		{"run ID", "run-abc-123"},
		{"dataset name", "eval-dataset"},
		{"metric name", "f1"},
		{"branch name", "keyword"},
		{"sample id", "s1"},
		{"correct icon", "✅"},
		{"incorrect icon", "❌"},
		{"error icon", "⚠️"},
		{"tag", "nightly"},
		{"per-label detail", "positive="},
		{"confusion value", "40"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("%s: output missing %q", c.name, c.needle)
		}
	}
}

// --- Table Reporter ---

func TestTableReporter(t *testing.T) {
	t.Parallel()
	r := Table()
	result := makeTestResult()

	if r.Name() != "table" {
		t.Errorf("Name() = %q, want table", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	checks := []struct {
		name, needle string
	}{
		{"header box", "╔══"},
		{"title", "BENCH RUN REPORT"},
		{"metrics section", "METRICS"},
		{"branches section", "BRANCHES"},
		{"samples section", "SAMPLES"},
		{"run ID", "run-abc-123"},
		{"dataset", "eval-dataset"},
		{"metric name", "f1"},
		{"branch name", "keyword"},
		{"sample id", "s1"},
		{"plus/minus separator", "+-"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("%s: output missing %q", c.name, c.needle)
		}
	}
}

// --- CSV Reporter ---

func TestCSVReporter(t *testing.T) {
	t.Parallel()
	r := CSV()
	result := makeTestResult()

	if r.Name() != "csv" {
		t.Errorf("Name() = %q, want csv", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Parse as CSV
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}

	// Header + 2 metric rows
	if len(records) != 3 {
		t.Fatalf("expected 3 rows (header + 2 metrics), got %d", len(records))
	}

	// Check header
	header := records[0]
	if header[0] != "metric_name" || header[1] != "value" || header[2] != "details" {
		t.Errorf("unexpected header: %v", header)
	}

	// Check metric rows
	if records[1][0] != "f1" {
		t.Errorf("first metric = %q, want f1", records[1][0])
	}
	if records[2][0] != "auc" {
		t.Errorf("second metric = %q, want auc", records[2][0])
	}

	// f1 should have per-label details
	if !strings.Contains(records[1][2], "positive=") {
		t.Error("f1 details should contain per-label values")
	}
}

// --- JUnit Reporter ---

func TestJUnitReporter(t *testing.T) {
	t.Parallel()
	r := JUnit()
	result := makeTestResult()

	if r.Name() != "junit" {
		t.Errorf("Name() = %q, want junit", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "<?xml") {
		t.Error("missing XML declaration")
	}
	if !strings.Contains(out, "<testsuites") {
		t.Error("missing <testsuites>")
	}

	// No targets → no test cases
	var suites junitTestSuites
	idx := strings.Index(out, "<testsuites")
	if idx < 0 {
		t.Fatal("missing <testsuites in output")
	}
	xmlBody := out[idx:]
	if err := xml.Unmarshal([]byte(xmlBody), &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites.Suites))
	}
	if suites.Suites[0].Tests != 0 {
		t.Errorf("expected 0 test cases without targets, got %d", suites.Suites[0].Tests)
	}

	// Check properties
	props := suites.Suites[0].Properties
	foundRunID := false
	for _, p := range props {
		if p.Name == "run_id" && p.Value == "run-abc-123" {
			foundRunID = true
		}
	}
	if !foundRunID {
		t.Error("missing run_id property")
	}
}

func TestJUnitReporterWithTargets(t *testing.T) {
	t.Parallel()
	result := makeTestResult()

	tests := []struct {
		name         string
		targets      map[string]float64
		wantTests    int
		wantFailures int
	}{
		{
			name:         "all pass",
			targets:      map[string]float64{"f1": 0.80, "auc": 0.90},
			wantTests:    2,
			wantFailures: 0,
		},
		{
			name:         "one fails",
			targets:      map[string]float64{"f1": 0.95, "auc": 0.90},
			wantTests:    2,
			wantFailures: 1,
		},
		{
			name:         "all fail",
			targets:      map[string]float64{"f1": 0.99, "auc": 0.99},
			wantTests:    2,
			wantFailures: 2,
		},
		{
			name:         "unmatched target ignored",
			targets:      map[string]float64{"nonexistent": 0.5},
			wantTests:    0,
			wantFailures: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := JUnit(WithTargets(tt.targets))

			var buf bytes.Buffer
			if err := r.Generate(&buf, result); err != nil {
				t.Fatalf("Generate() error: %v", err)
			}
			out := buf.String()
			idx := strings.Index(out, "<testsuites")
			if idx < 0 {
				t.Fatal("missing <testsuites in output")
			}
			xmlBody := out[idx:]

			var suites junitTestSuites
			if err := xml.Unmarshal([]byte(xmlBody), &suites); err != nil {
				t.Fatalf("invalid XML: %v", err)
			}
			suite := suites.Suites[0]
			if suite.Tests != tt.wantTests {
				t.Errorf("tests = %d, want %d", suite.Tests, tt.wantTests)
			}
			if suite.Failures != tt.wantFailures {
				t.Errorf("failures = %d, want %d", suite.Failures, tt.wantFailures)
			}

			// Verify failure messages contain actual values
			for _, tc := range suite.TestCases {
				if tc.Failure != nil {
					if !strings.Contains(tc.Failure.Message, tc.Name) {
						t.Errorf("failure message should contain metric name %q", tc.Name)
					}
				}
			}
		})
	}
}

// --- VegaLite Reporter ---

func TestVegaLiteReporter(t *testing.T) {
	t.Parallel()
	r := VegaLite()
	result := makeTestResult()

	if r.Name() != "vegalite" {
		t.Errorf("Name() = %q, want vegalite", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have multiple spec files
	if len(parsed) == 0 {
		t.Fatal("expected non-empty specs map")
	}

	expectedKeys := []string{
		"roc_curve.vl.json",
		"confusion_matrix.vl.json",
		"calibration.vl.json",
		"branch_comparison.vl.json",
	}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing expected spec %q", key)
		}
	}
}

func TestVegaLiteSpecs(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	specs := VegaLiteSpecs(result)

	for name, spec := range specs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			t.Errorf("%s: spec is not a map", name)
			continue
		}
		schema, ok := specMap["$schema"].(string)
		if !ok || schema != vegaLiteSchema {
			t.Errorf("%s: $schema = %v, want %q", name, specMap["$schema"], vegaLiteSchema)
		}
		if _, ok := specMap["title"]; !ok {
			t.Errorf("%s: missing title", name)
		}
	}
}

func TestVegaLiteSpecsROC(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	specs := VegaLiteSpecs(result)

	spec, ok := specs["roc_curve.vl.json"]
	if !ok {
		t.Fatal("missing roc_curve spec")
	}
	specMap := spec.(map[string]any)
	title, _ := specMap["title"].(string)
	if !strings.Contains(title, "ROC") {
		t.Errorf("title = %q, expected to contain ROC", title)
	}
	if !strings.Contains(title, "0.92") {
		t.Errorf("title = %q, expected to contain AUC value", title)
	}
}

func TestVegaLiteSpecsConfusionMatrix(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	specs := VegaLiteSpecs(result)

	spec, ok := specs["confusion_matrix.vl.json"]
	if !ok {
		t.Fatal("missing confusion_matrix spec")
	}
	specMap := spec.(map[string]any)
	if specMap["title"] != "Confusion Matrix" {
		t.Errorf("title = %v, want Confusion Matrix", specMap["title"])
	}
}

func TestVegaLiteSpecsBranchComparison(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	specs := VegaLiteSpecs(result)

	if _, ok := specs["branch_comparison.vl.json"]; !ok {
		t.Fatal("missing branch_comparison spec")
	}
}

func TestVegaLiteSpecsScoreDistribution(t *testing.T) {
	t.Parallel()
	// Test with samples but no explicit score_distribution curve
	result := makeTestResult()
	result.Curves = nil // Remove explicit curves; should fall back to samples
	specs := VegaLiteSpecs(result)

	if _, ok := specs["score_distribution.vl.json"]; !ok {
		t.Fatal("missing score_distribution spec from samples fallback")
	}
}

func TestVegaLiteSpecsEmpty(t *testing.T) {
	t.Parallel()
	result := makeEmptyResult()
	specs := VegaLiteSpecs(result)
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for empty result, got %d", len(specs))
	}
}

// --- HTML Reporter ---

func TestHTMLReporter(t *testing.T) {
	t.Parallel()
	r := HTML()
	result := makeTestResult()

	if r.Name() != "html" {
		t.Errorf("Name() = %q, want html", r.Name())
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	checks := []struct {
		name, needle string
	}{
		{"doctype", "<!DOCTYPE html>"},
		{"html tag", "<html"},
		{"closing html", "</html>"},
		{"vega script", "vega-embed"},
		{"title", "Bench Report"},
		{"tag in title", "nightly"},
		{"run ID", "run-abc-123"},
		{"dataset name", "eval-dataset"},
		{"summary section", "Summary"},
		{"metrics section", "Metrics"},
		{"charts section", "Charts"},
		{"branch section", "Branch Comparison"},
		{"sample section", "Sample Details"},
		{"chart div", "chart-"},
		{"vegaEmbed call", "vegaEmbed"},
		{"metric value", "0.88"},
		{"sample id", "s1"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("%s: output missing %q", c.name, c.needle)
		}
	}
}

func TestHTMLReporterWithEmptyCharts(t *testing.T) {
	t.Parallel()
	r := HTML()
	result := makeMinimalResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "<html") {
		t.Error("missing <html> tag")
	}
	// No charts section expected
	if strings.Contains(out, "Charts") {
		t.Error("should not have Charts section with no curve data")
	}
}

// --- Interface compliance ---

func TestAllReportersInterface(t *testing.T) {
	t.Parallel()

	reporters := []struct {
		name     string
		reporter Reporter
	}{
		{"json", JSON()},
		{"markdown", Markdown()},
		{"table", Table()},
		{"csv", CSV()},
		{"junit", JUnit()},
		{"vegalite", VegaLite()},
		{"html", HTML()},
	}

	for _, rr := range reporters {
		t.Run(rr.name, func(t *testing.T) {
			t.Parallel()
			if rr.reporter.Name() != rr.name {
				t.Errorf("Name() = %q, want %q", rr.reporter.Name(), rr.name)
			}
		})
	}
}

// --- Robustness: empty and minimal results ---

func TestReportersWithEmptyResult(t *testing.T) {
	t.Parallel()

	reporters := []Reporter{
		JSON(), Markdown(), Table(), CSV(), JUnit(), VegaLite(), HTML(),
	}
	result := makeEmptyResult()

	for _, r := range reporters {
		t.Run(r.Name(), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := r.Generate(&buf, result); err != nil {
				t.Fatalf("%s: Generate() error: %v", r.Name(), err)
			}
			if buf.Len() == 0 {
				t.Fatalf("%s: empty output", r.Name())
			}
		})
	}
}

func TestReportersWithMinimalResult(t *testing.T) {
	t.Parallel()

	reporters := []Reporter{
		JSON(), Markdown(), Table(), CSV(), JUnit(), VegaLite(), HTML(),
	}
	result := makeMinimalResult()

	for _, r := range reporters {
		t.Run(r.Name(), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			if err := r.Generate(&buf, result); err != nil {
				t.Fatalf("%s: Generate() error: %v", r.Name(), err)
			}
			if buf.Len() == 0 {
				t.Fatalf("%s: empty output", r.Name())
			}
		})
	}
}

// --- Edge cases ---

func TestCSVReporterNoMetrics(t *testing.T) {
	t.Parallel()
	r := CSV()
	result := makeEmptyResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}
	// Should have only header
	if len(records) != 1 {
		t.Errorf("expected 1 row (header only), got %d", len(records))
	}
}

func TestMarkdownWithoutTag(t *testing.T) {
	t.Parallel()
	r := Markdown()
	result := makeTestResult()
	result.Tag = ""

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "**Tag**") {
		t.Error("tag row should not appear when tag is empty")
	}
}

func TestTableTruncation(t *testing.T) {
	t.Parallel()
	r := Table()
	result := makeTestResult()

	// Add enough samples to trigger truncation
	samples := make([]bench.SampleResult, 30)
	for i := range samples {
		samples[i] = bench.SampleResult{
			ID:    "sample-" + string(rune('A'+i%26)),
			Label: "positive", Predicted: "positive",
			Score: 0.9, Correct: true,
		}
	}
	result.Samples = samples

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "showing") {
		t.Error("expected truncation message for >20 samples")
	}
}

func TestMarkdownTruncation(t *testing.T) {
	t.Parallel()
	r := Markdown()
	result := makeTestResult()

	samples := make([]bench.SampleResult, 60)
	for i := range samples {
		samples[i] = bench.SampleResult{
			ID: "sample", Label: "positive", Predicted: "positive",
			Score: 0.9, Correct: true,
		}
	}
	result.Samples = samples

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if !strings.Contains(buf.String(), "Showing") {
		t.Error("expected truncation message for >50 samples")
	}
}

func TestJUnitWithTag(t *testing.T) {
	t.Parallel()
	r := JUnit()
	result := makeTestResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if !strings.Contains(buf.String(), "nightly") {
		t.Error("JUnit output should contain tag in properties")
	}
}

func TestJUnitWithoutTag(t *testing.T) {
	t.Parallel()
	r := JUnit()
	result := makeTestResult()
	result.Tag = ""

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Should not have tag property
	out := buf.String()
	idx := strings.Index(out, "<testsuites")
	if idx < 0 {
		t.Fatal("missing <testsuites in output")
	}
	xmlBody := out[idx:]
	var suites junitTestSuites
	if err := xml.Unmarshal([]byte(xmlBody), &suites); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	for _, p := range suites.Suites[0].Properties {
		if p.Name == "tag" {
			t.Error("tag property should not be present when tag is empty")
		}
	}
}

func TestVegaLiteCalibrationSpec(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	specs := VegaLiteSpecs(result)

	spec, ok := specs["calibration.vl.json"]
	if !ok {
		t.Fatal("missing calibration spec")
	}
	specMap := spec.(map[string]any)
	if specMap["title"] != "Calibration Curve" {
		t.Errorf("title = %v, want Calibration Curve", specMap["title"])
	}
}

func TestVegaLiteThresholdSweep(t *testing.T) {
	t.Parallel()
	result := makeTestResult()
	// ThresholdPoints are stored as []any in curves map; findAllCurves checks []any
	// We need them as direct slice or via Metrics Detail.
	// Put them in a metric detail as a slice of ThresholdPoint.
	result.Metrics = append(result.Metrics, bench.MetricResult{
		Name:  "threshold_sweep",
		Value: 0,
		Detail: []bench.ThresholdPoint{
			{Threshold: 0.3, Precision: 0.70, Recall: 0.95, F1: 0.81, Accuracy: 0.75},
			{Threshold: 0.5, Precision: 0.85, Recall: 0.80, F1: 0.82, Accuracy: 0.84},
		},
	})
	specs := VegaLiteSpecs(result)

	if _, ok := specs["threshold_sweep.vl.json"]; !ok {
		t.Fatal("missing threshold_sweep spec")
	}
}

func TestHTMLEscaping(t *testing.T) {
	t.Parallel()
	r := HTML()
	result := makeTestResult()
	result.Tag = "test<script>alert(1)</script>"

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "<script>alert(1)") {
		t.Error("HTML output should escape special characters in tag")
	}
}

func TestJSONBranchSorting(t *testing.T) {
	t.Parallel()
	r := JSON()
	result := makeTestResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(buf.Bytes(), &parsed)

	branches := parsed["branches"].([]any)
	first := branches[0].(map[string]any)["name"].(string)
	second := branches[1].(map[string]any)["name"].(string)

	if first > second {
		t.Errorf("branches not sorted: %q before %q", first, second)
	}
}
