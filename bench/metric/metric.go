package metric

import "github.com/kbukum/gokit/bench"

// Metric computes evaluation scores from predictions vs ground truth.
type Metric[L comparable] interface {
	Name() string
	Compute(scored []bench.ScoredSample[L]) Result
}

// Result holds a metric computation's output.
type Result struct {
	Name   string             `json:"name"`
	Value  float64            `json:"value"`
	Values map[string]float64 `json:"values,omitempty"`
	Detail any                `json:"detail,omitempty"`
}

// Suite groups multiple metrics for batch evaluation.
type Suite[L comparable] struct {
	metrics []Metric[L]
}

// NewSuite creates a metric suite.
func NewSuite[L comparable](metrics ...Metric[L]) *Suite[L] {
	return &Suite[L]{metrics: metrics}
}

// Add appends metrics to the suite.
func (s *Suite[L]) Add(metrics ...Metric[L]) {
	s.metrics = append(s.metrics, metrics...)
}

// Compute runs all metrics and returns results.
func (s *Suite[L]) Compute(scored []bench.ScoredSample[L]) []Result {
	results := make([]Result, 0, len(s.metrics))
	for _, m := range s.metrics {
		results = append(results, m.Compute(scored))
	}
	return results
}

// Metrics returns the list of metrics in the suite.
func (s *Suite[L]) Metrics() []Metric[L] {
	return s.metrics
}
