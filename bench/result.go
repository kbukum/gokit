package bench

import "time"

// RunResult holds the complete output of a benchmark run. It serializes with the shared cross-kit
// envelope — a top-level `$schema` URL and `version` (see [SchemaURL] and [SchemaVersion]) — so a
// run persisted by any sibling kit decodes in the others. The envelope keys are emitted from the
// package constants at marshal time rather than stored on the struct.
type RunResult struct {
	ID        string                  `json:"id"`
	Timestamp time.Time               `json:"timestamp"`
	Tag       string                  `json:"tag,omitempty"`
	Duration  time.Duration           `json:"duration_ms"`
	Dataset   DatasetInfo             `json:"dataset"`
	Metrics   []MetricResult          `json:"metrics"`
	Branches  map[string]BranchResult `json:"branches"`
	Samples   []SampleResult          `json:"samples"`
	Curves    map[string]any          `json:"curves,omitempty"`
	// Provenance records reproducibility metadata: seed, source commit, tool/host
	// identity, and an order-independent dataset content hash.
	Provenance RunProvenance `json:"provenance"`
}

// DatasetInfo holds summary info about the dataset used.
type DatasetInfo struct {
	Name              string         `json:"name"`
	Version           string         `json:"version"`
	SampleCount       int            `json:"sample_count"`
	LabelDistribution map[string]int `json:"label_distribution"`
}

// MetricResult pairs a metric name with its result.
type MetricResult struct {
	Name   string             `json:"name"`
	Value  float64            `json:"value"`
	Values map[string]float64 `json:"values,omitempty"`
	Detail any                `json:"detail,omitempty"`
	// Direction is the optimization direction of Value and of every entry in
	// Values not overridden in Directions: whether higher or lower is better, or
	// whether the metric is purely descriptive. RunComparator uses it to classify
	// a change as an improvement or a regression. The zero value is HigherIsBetter,
	// so accuracy-style metrics need not set it explicitly.
	Direction Direction `json:"direction"`
	// Directions overrides the optimization direction of individual Values
	// entries whose direction differs from the metric's top-level Direction. A key
	// absent from this map inherits Direction. RunComparator resolves each
	// subvalue's direction through it, so a heterogeneous metric — a
	// higher-is-better headline (F1, R²) alongside lower-is-better diagnostics
	// (false-positive rate, residual sum of squares) — classifies every subvalue
	// correctly instead of inheriting one direction for the whole map.
	Directions map[string]Direction `json:"directions,omitempty"`
}

// BranchResult holds results for a single evaluator branch.
type BranchResult struct {
	Name             string             `json:"name"`
	Tier             int                `json:"tier"`
	Metrics          map[string]float64 `json:"metrics"`
	AvgScorePositive float64            `json:"avg_score_positive"`
	AvgScoreNegative float64            `json:"avg_score_negative"`
	Duration         time.Duration      `json:"duration_ms"`
	Errors           int                `json:"errors"`
}

// SampleResult holds per-sample evaluation results.
type SampleResult struct {
	ID           string             `json:"id"`
	Label        string             `json:"label"`
	Predicted    string             `json:"predicted"`
	Score        float64            `json:"score"`
	Correct      bool               `json:"correct"`
	BranchScores map[string]float64 `json:"branch_scores,omitempty"`
	Duration     time.Duration      `json:"duration_ms"`
	Error        string             `json:"error,omitempty"`
}

// RunSummary is a lightweight summary for listing runs.
type RunSummary struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Tag       string    `json:"tag,omitempty"`
	Dataset   string    `json:"dataset"`
	F1        float64   `json:"f1,omitempty"`
}
