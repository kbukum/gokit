package bench

import "time"

// RunResult holds bounded run summaries in the SchemaVersion 2.0 envelope. Detailed records are read through ResultStore; sibling-kit v2 interchange requires their corresponding schema update.
type RunResult struct {
	ID                   string                  `json:"id"`
	Timestamp            time.Time               `json:"timestamp"`
	Tag                  string                  `json:"tag,omitempty"`
	Duration             time.Duration           `json:"duration_ms"`
	Dataset              DatasetInfo             `json:"dataset"`
	Metrics              []MetricResult          `json:"metrics"`
	Branches             map[string]BranchResult `json:"branches"`
	ROC                  *ROCCurve               `json:"roc,omitempty"`
	Confusion            *ConfusionMatrixDetail  `json:"confusion,omitempty"`
	Calibration          *CalibrationCurve       `json:"calibration,omitempty"`
	ThresholdSweep       []ThresholdPoint        `json:"threshold_sweep,omitempty"`
	ScoreDistributions   []ScoreDistribution     `json:"score_distributions,omitempty"`
	DistributionsOmitted bool                    `json:"distributions_omitted,omitempty"`
	// Provenance records reproducibility metadata and an order-dependent dataset hash.
	Provenance            RunProvenance      `json:"provenance"`
	Evaluation            EvaluationIdentity `json:"evaluation"`
	EvaluationFingerprint string             `json:"evaluation_fingerprint"`
	RecordSegments        []int              `json:"record_segments"`
	RecordReadLimit       int64              `json:"record_read_limit"`
}

// DatasetInfo holds summary info about the dataset used.
type DatasetInfo struct {
	DistributionOmitted bool           `json:"distribution_omitted,omitempty"`
	Name                string         `json:"name"`
	Version             string         `json:"version"`
	SampleCount         int            `json:"sample_count"`
	LabelDistribution   map[string]int `json:"label_distribution"`
}

// MetricResult pairs a metric name with its result.
type MetricResult struct {
	Name           string                 `json:"name"`
	Value          float64                `json:"value"`
	Values         map[string]float64     `json:"values,omitempty"`
	Judge          *JudgeProvenance       `json:"judge,omitempty"`
	Confusion      *ConfusionMatrixDetail `json:"confusion,omitempty"`
	ROC            *ROCCurve              `json:"roc,omitempty"`
	Calibration    *CalibrationCurve      `json:"calibration,omitempty"`
	ThresholdSweep []ThresholdPoint       `json:"threshold_sweep,omitempty"`
	Components     []MetricResult         `json:"components,omitempty"`
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
	MetricResults      []MetricResult     `json:"metric_results"`
	Performance        PerformanceSummary `json:"performance"`
	Subject            SubjectIdentity    `json:"subject"`
	SubjectFingerprint string             `json:"subject_fingerprint"`
	Name               string             `json:"name"`
	Tier               int                `json:"tier"`
	Metrics            map[string]float64 `json:"metrics"`
	AvgScorePositive   float64            `json:"avg_score_positive"`
	AvgScoreNegative   float64            `json:"avg_score_negative"`
	Duration           time.Duration      `json:"duration_ms"`
	Errors             int                `json:"errors"`
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
