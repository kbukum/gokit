package report

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
)

func sampleRunResult() *bench.RunResult {
	return &bench.RunResult{
		ID:        "test-run-001",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Tag:       "v1-test",
		Duration:  2 * time.Second,
		Dataset: bench.DatasetInfo{
			Name:              "test-dataset",
			Version:           "1.0",
			SampleCount:       3,
			LabelDistribution: map[string]int{"positive": 2, "negative": 1},
		},
		Metrics: []bench.MetricResult{
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
		Branches: map[string]bench.BranchResult{
			"main": {
				Name:             "main",
				Tier:             0,
				Metrics:          map[string]float64{"f1": 0.85},
				AvgScorePositive: 0.9,
				AvgScoreNegative: 0.3,
				Duration:         time.Second,
				Errors:           0,
			},
		},
		Samples: []bench.SampleResult{
			{ID: "s1", Label: "positive", Predicted: "positive", Score: 0.95, Correct: true},
			{ID: "s2", Label: "positive", Predicted: "positive", Score: 0.80, Correct: true},
			{ID: "s3", Label: "negative", Predicted: "positive", Score: 0.60, Correct: false},
		},
	}
}

func TestJSONReporterName(t *testing.T) {
	t.Parallel()

	r := JSON()
	if r.Name() != "json" {
		t.Errorf("Name() = %q, want %q", r.Name(), "json")
	}
}

func TestJSONReporterGenerate(t *testing.T) {
	t.Parallel()

	r := JSON()
	result := sampleRunResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	output := buf.Bytes()
	if len(output) == 0 {
		t.Fatal("Generate() produced empty output")
	}

	// Should be valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check $schema and version fields.
	if schema, ok := parsed["$schema"].(string); !ok || schema == "" {
		t.Error("missing or empty $schema field")
	}
	if version, ok := parsed["version"].(string); !ok || version == "" {
		t.Error("missing or empty version field")
	}
}

func TestJSONReporterRoundTrip(t *testing.T) {
	t.Parallel()

	r := JSON()
	result := sampleRunResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Verify run section.
	run, ok := parsed["run"].(map[string]any)
	if !ok {
		t.Fatal("missing 'run' section")
	}
	if run["id"] != "test-run-001" {
		t.Errorf("run.id = %v, want %q", run["id"], "test-run-001")
	}

	// Verify dataset section.
	ds, ok := parsed["dataset"].(map[string]any)
	if !ok {
		t.Fatal("missing 'dataset' section")
	}
	if ds["name"] != "test-dataset" {
		t.Errorf("dataset.name = %v, want %q", ds["name"], "test-dataset")
	}

	// Verify metrics section.
	metrics, ok := parsed["metrics"].([]any)
	if !ok {
		t.Fatal("missing 'metrics' section")
	}
	if len(metrics) != 1 {
		t.Errorf("len(metrics) = %d, want 1", len(metrics))
	}

	// Verify samples section.
	samples, ok := parsed["samples"].([]any)
	if !ok {
		t.Fatal("missing 'samples' section")
	}
	if len(samples) != 3 {
		t.Errorf("len(samples) = %d, want 3", len(samples))
	}
}

func TestJSONReporterSchemaAndVersion(t *testing.T) {
	t.Parallel()

	r := JSON()
	result := sampleRunResult()

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal(buf.Bytes(), &parsed)

	schema := parsed["$schema"].(string)
	if schema != bench.SchemaURL {
		t.Errorf("$schema = %q, want %q", schema, bench.SchemaURL)
	}

	version := parsed["version"].(string)
	if version != bench.SchemaVersion {
		t.Errorf("version = %q, want %q", version, bench.SchemaVersion)
	}
}

func TestJSONReporterEmptyResult(t *testing.T) {
	t.Parallel()

	r := JSON()
	result := &bench.RunResult{
		ID:        "empty-run",
		Schema:    bench.SchemaVersion,
		Timestamp: time.Now(),
	}

	var buf bytes.Buffer
	if err := r.Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}
