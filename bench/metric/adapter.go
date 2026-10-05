package metric

import "github.com/kbukum/gokit/bench"

// AsRunMetric preserves optional streaming capabilities without an adapter.
func AsRunMetric[L comparable](m Metric[L]) bench.RunMetric[L] { return m }

// AsRunMetrics preserves metric order and optional streaming capabilities.
func AsRunMetrics[L comparable](metrics ...Metric[L]) []bench.RunMetric[L] {
	out := make([]bench.RunMetric[L], len(metrics))
	for i, m := range metrics {
		out[i] = m
	}
	return out
}
