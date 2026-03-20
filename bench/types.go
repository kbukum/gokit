package bench

// Sample represents a labeled data point in an evaluation dataset.
type Sample[L comparable] struct {
	ID       string
	Input    []byte
	Label    L
	Source   string
	Metadata map[string]any
}

// Prediction represents an evaluator's output for a single sample.
type Prediction[L comparable] struct {
	SampleID string
	Label    L
	Score    float64
	Scores   map[L]float64
	Metadata map[string]any
}

// ScoredSample pairs a ground-truth sample with its prediction.
type ScoredSample[L comparable] struct {
	Sample     Sample[L]
	Prediction Prediction[L]
}

// LabelMapper converts a string label from a manifest into a typed label.
type LabelMapper[L comparable] func(string) (L, error)
