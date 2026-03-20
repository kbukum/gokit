package bench

import "time"

// RunMetric computes evaluation scores from predictions vs ground truth.
// This interface mirrors metric.Metric[L] but lives in bench to avoid
// an import cycle (bench/metric already imports bench).
// Use metric.AsRunMetric to adapt metric.Metric[L] values.
type RunMetric[L comparable] interface {
	Name() string
	Compute(scored []ScoredSample[L]) MetricResult
}

// RunOption configures a BenchRunner.
type RunOption[L comparable] func(*runConfig[L])

type runConfig[L comparable] struct {
	metrics          []RunMetric[L]
	storage          RunStorage
	concurrency      int
	timeout          time.Duration
	tag              string
	targets          map[string]float64
	failOnRegression bool
}

func defaultConfig[L comparable]() runConfig[L] {
	return runConfig[L]{
		concurrency: 1,
	}
}

// WithMetrics configures the metrics to compute.
func WithMetrics[L comparable](metrics ...RunMetric[L]) RunOption[L] {
	return func(c *runConfig[L]) {
		c.metrics = append(c.metrics, metrics...)
	}
}

// WithStorage configures the storage backend for persisting results.
func WithStorage[L comparable](s RunStorage) RunOption[L] {
	return func(c *runConfig[L]) {
		c.storage = s
	}
}

// WithConcurrency sets the number of parallel evaluation workers.
// Values <= 1 mean sequential execution.
func WithConcurrency[L comparable](n int) RunOption[L] {
	return func(c *runConfig[L]) {
		if n < 1 {
			n = 1
		}
		c.concurrency = n
	}
}

// WithTimeout sets the per-sample evaluation timeout.
func WithTimeout[L comparable](d time.Duration) RunOption[L] {
	return func(c *runConfig[L]) {
		c.timeout = d
	}
}

// WithTag sets a human-readable tag for the run.
func WithTag[L comparable](tag string) RunOption[L] {
	return func(c *runConfig[L]) {
		c.tag = tag
	}
}

// WithTargets sets metric target thresholds (metric name → minimum value).
func WithTargets[L comparable](targets map[string]float64) RunOption[L] {
	return func(c *runConfig[L]) {
		c.targets = targets
	}
}

// WithFailOnRegression configures whether the run should fail if a
// regression is detected compared to the previous run.
func WithFailOnRegression[L comparable](b bool) RunOption[L] {
	return func(c *runConfig[L]) {
		c.failOnRegression = b
	}
}
