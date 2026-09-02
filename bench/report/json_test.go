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
				Name:      "classification",
				Value:     0.85,
				Direction: bench.Neutral,
				Values: map[string]float64{
					"precision": 0.9,
					"recall":    0.8,
					"f1":        0.85,
				},
			},
		},
		Provenance: bench.RunProvenance{
			Seed:         99,
			RNGAlgorithm: bench.RNGAlgorithm,
			DatasetHash:  "deadbeef",
			Metrics:      []string{"classification"},
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

	// Verify run identity is flat at the top level.
	if parsed["id"] != "test-run-001" {
		t.Errorf("id = %v, want %q", parsed["id"], "test-run-001")
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
	metric, ok := metrics[0].(map[string]any)
	if !ok {
		t.Fatal("metrics[0] is not an object")
	}
	if dir, ok := metric["direction"].(string); !ok || dir != "neutral" {
		t.Errorf("metrics[0].direction = %v (present=%v), want %q", metric["direction"], ok, "neutral")
	}
	if _, ok := metric["descriptive"]; ok {
		t.Errorf("metrics[0].descriptive present, want it removed in favor of direction")
	}

	// Verify provenance is preserved by JSON reporter projection.
	prov, ok := parsed["provenance"].(map[string]any)
	if !ok {
		t.Fatal("missing 'provenance' section")
	}
	if got := prov["seed"]; got != float64(99) {
		t.Errorf("provenance.seed = %v, want 99", got)
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

// TestJSONReporterSchemaContract locks the shared cross-kit contract: the
// serialized run carries the agreed $schema URL and version, and a metric
// serializes its direction with no legacy descriptive key. It fails on any
// drift from the value rskit's bench schema also emits.
func TestJSONReporterSchemaContract(t *testing.T) {
	t.Parallel()

	if bench.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %q, want %q (cross-kit contract)", bench.SchemaVersion, "1.0")
	}
	if bench.SchemaURL != "https://gokit.dev/bench/v1/schema.json" {
		t.Errorf("SchemaURL = %q, want %q (cross-kit contract)", bench.SchemaURL, "https://gokit.dev/bench/v1/schema.json")
	}

	result := &bench.RunResult{
		ID:        "contract-run",
		Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Metrics: []bench.MetricResult{
			{Name: "mae", Value: 0.25, Direction: bench.LowerIsBetter},
			{Name: "token_stats", Value: 100, Direction: bench.Neutral},
		},
	}

	var buf bytes.Buffer
	if err := JSON().Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Metrics []struct {
			Name      string `json:"name"`
			Direction string `json:"direction"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if parsed.Schema != "https://gokit.dev/bench/v1/schema.json" {
		t.Errorf("$schema = %q, want the cross-kit URL", parsed.Schema)
	}
	if parsed.Version != "1.0" {
		t.Errorf("version = %q, want %q", parsed.Version, "1.0")
	}
	want := map[string]string{"mae": "lower_is_better", "token_stats": "neutral"}
	for _, m := range parsed.Metrics {
		if got := want[m.Name]; got != m.Direction {
			t.Errorf("metric %q direction = %q, want %q", m.Name, m.Direction, got)
		}
	}

	// The legacy descriptive key must be gone entirely from the emitted contract.
	if bytes.Contains(buf.Bytes(), []byte("descriptive")) {
		t.Errorf("emitted JSON contains legacy 'descriptive' key:\n%s", buf.String())
	}
}

// TestJSONReporterPerKeyDirections verifies that per-key subvalue directions are
// serialized under a "directions" object and round-trip, and that a metric
// without overrides omits the object entirely.
func TestJSONReporterPerKeyDirections(t *testing.T) {
	t.Parallel()

	result := &bench.RunResult{
		ID:        "dir-run",
		Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Metrics: []bench.MetricResult{
			{
				Name:      "classification",
				Value:     0.80,
				Direction: bench.HigherIsBetter,
				Values:    map[string]float64{"f1": 0.80, "fpr": 0.10},
				Directions: map[string]bench.Direction{
					"fpr": bench.LowerIsBetter,
				},
			},
			{Name: "accuracy", Value: 0.9, Direction: bench.HigherIsBetter},
		},
	}

	var buf bytes.Buffer
	if err := JSON().Generate(&buf, result); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	var parsed struct {
		Metrics []struct {
			Name       string                     `json:"name"`
			Directions map[string]bench.Direction `json:"directions"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	byName := map[string]map[string]bench.Direction{}
	for _, m := range parsed.Metrics {
		byName[m.Name] = m.Directions
	}
	if got := byName["classification"]["fpr"]; got != bench.LowerIsBetter {
		t.Errorf("classification.fpr direction = %v, want LowerIsBetter", got)
	}
	if _, ok := byName["classification"]["f1"]; ok {
		t.Error("f1 should inherit the top-level direction, not appear in directions")
	}
	if byName["accuracy"] != nil {
		t.Error("a metric without overrides must omit the directions object")
	}
}

func TestJSONReporterEmptyResult(t *testing.T) {
	t.Parallel()

	r := JSON()
	result := &bench.RunResult{
		ID:        "empty-run",
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
