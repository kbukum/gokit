package bench

import "time"

// RunResult holds the complete output of a benchmark run.
type RunResult struct {
	ID        string                  `json:"id"`
	Schema    string                  `json:"schema"`
	Timestamp time.Time               `json:"timestamp"`
	Tag       string                  `json:"tag,omitempty"`
	Duration  time.Duration           `json:"duration_ms"`
	Dataset   DatasetInfo             `json:"dataset"`
	Metrics   []MetricResult          `json:"metrics"`
	Branches  map[string]BranchResult `json:"branches"`
	Samples   []SampleResult          `json:"samples"`
	Curves    map[string]any          `json:"curves,omitempty"`
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
