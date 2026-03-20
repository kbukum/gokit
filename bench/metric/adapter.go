package metric

import "github.com/kbukum/gokit/bench"

// runMetricAdapter adapts a metric.Metric[L] to a bench.RunMetric[L].
type runMetricAdapter[L comparable] struct {
	m Metric[L]
}

// AsRunMetric converts a metric.Metric[L] into a bench.RunMetric[L]
// for use with BenchRunner.
func AsRunMetric[L comparable](m Metric[L]) bench.RunMetric[L] {
	return &runMetricAdapter[L]{m: m}
}

// AsRunMetrics converts multiple metric.Metric[L] values into bench.RunMetric[L].
func AsRunMetrics[L comparable](metrics ...Metric[L]) []bench.RunMetric[L] {
	out := make([]bench.RunMetric[L], len(metrics))
	for i, m := range metrics {
		out[i] = AsRunMetric[L](m)
	}
	return out
}

func (a *runMetricAdapter[L]) Name() string {
	return a.m.Name()
}

func (a *runMetricAdapter[L]) Compute(scored []bench.ScoredSample[L]) bench.MetricResult {
	r := a.m.Compute(scored)
	return bench.MetricResult{
		Name:   r.Name,
		Value:  r.Value,
		Values: r.Values,
		Detail: r.Detail,
	}
}
