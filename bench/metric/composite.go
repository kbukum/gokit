package metric

import (
	"fmt"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// Weighted creates a composite metric that combines multiple metrics with weights.
// The composite Value is the weighted sum of individual metric Values.
func Weighted[L comparable](weights map[Metric[L]]float64) Metric[L] {
	entries := make([]weightedEntry[L], 0, len(weights))
	for m, w := range weights {
		entries = append(entries, weightedEntry[L]{metric: m, weight: w})
	}
	return &weightedMetric[L]{entries: entries}
}

type weightedEntry[L comparable] struct {
	metric Metric[L]
	weight float64
}

type weightedMetric[L comparable] struct {
	entries []weightedEntry[L]
}

func (m *weightedMetric[L]) Name() string {
	names := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		names = append(names, fmt.Sprintf("%s*%.2f", e.metric.Name(), e.weight))
	}
	return "weighted(" + strings.Join(names, "+") + ")"
}

func (m *weightedMetric[L]) Compute(scored []bench.ScoredSample[L]) Result {
	values := make(map[string]float64)
	details := make([]Result, 0, len(m.entries))
	composite := 0.0

	for _, e := range m.entries {
		r := e.metric.Compute(scored)
		composite += r.Value * e.weight
		values[r.Name] = r.Value
		details = append(details, r)
	}

	return Result{
		Name:   m.Name(),
		Value:  composite,
		Values: values,
		Detail: details,
	}
}
