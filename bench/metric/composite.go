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
	directions := make(map[string]bench.Direction)
	details := make([]Result, 0, len(m.entries))
	composite := 0.0

	// effective collects each nonzero-weight component's contribution direction —
	// its own direction, flipped when the weight is negative — to derive the
	// composite's headline direction after the sum.
	effective := make([]bench.Direction, 0, len(m.entries))
	for _, e := range m.entries {
		r := e.metric.Compute(scored)
		composite += r.Value * e.weight
		values[r.Name] = r.Value
		directions[r.Name] = r.Direction
		details = append(details, r)
		if e.weight != 0 {
			eff := r.Direction
			if e.weight < 0 {
				eff = flipDirection(eff)
			}
			effective = append(effective, eff)
		}
	}

	return Result{
		Name:  m.Name(),
		Value: composite,
		// Each constituent value keeps its own direction; the composite's headline
		// direction is derived from how the weighted sum moves.
		Direction:  compositeDirection(effective),
		Values:     values,
		Directions: directions,
		Detail:     details,
	}
}

// compositeDirection derives the optimization direction of a weighted sum from
// the contribution directions of its nonzero-weight components. If any component
// is descriptive, or the components disagree on direction, the sum has no single
// optimization direction and is [bench.Neutral].
func compositeDirection(effective []bench.Direction) bench.Direction {
	var resolved bench.Direction
	seen := false
	for _, eff := range effective {
		if eff == bench.Neutral {
			return bench.Neutral
		}
		if !seen {
			resolved = eff
			seen = true
			continue
		}
		if eff != resolved {
			return bench.Neutral
		}
	}
	return resolved
}

// flipDirection swaps higher-is-better and lower-is-better; a neutral direction
// is unchanged.
func flipDirection(d bench.Direction) bench.Direction {
	switch d {
	case bench.HigherIsBetter:
		return bench.LowerIsBetter
	case bench.LowerIsBetter:
		return bench.HigherIsBetter
	default:
		return d
	}
}
