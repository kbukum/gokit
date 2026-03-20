package bench

import (
	"fmt"
	"math"
	"strings"
)

// RunComparator compares two benchmark runs.
type RunComparator struct {
	threshold float64
}

// CompareOption configures comparison.
type CompareOption func(*RunComparator)

// WithChangeThreshold sets the minimum absolute change to report as significant (default: 0.01).
func WithChangeThreshold(t float64) CompareOption {
	return func(c *RunComparator) { c.threshold = t }
}

// NewRunComparator creates a comparator with default settings.
func NewRunComparator(opts ...CompareOption) *RunComparator {
	c := &RunComparator{threshold: 0.01}
	for _, o := range opts {
		o(c)
	}
	return c
}

// RunDiff holds the comparison result between two benchmark runs.
type RunDiff struct {
	BaseID    string
	TargetID  string
	Changes   []MetricChange
	Fixed     []string // sample IDs that went from wrong to correct
	Regressed []string // sample IDs that went from correct to wrong
}

// MetricChange represents a change in a metric between two runs.
type MetricChange struct {
	Name        string
	OldValue    float64
	NewValue    float64
	Delta       float64
	Improved    bool
	Significant bool // above threshold
}

// Compare compares two RunResults and returns the diff.
func (c *RunComparator) Compare(base, target *RunResult) *RunDiff {
	diff := &RunDiff{
		BaseID:   base.ID,
		TargetID: target.ID,
	}

	// Compare top-level metrics.
	baseMetrics := make(map[string]float64, len(base.Metrics))
	for _, m := range base.Metrics {
		baseMetrics[m.Name] = m.Value
		for k, v := range m.Values {
			baseMetrics[k] = v
		}
	}

	seen := make(map[string]bool)
	for _, m := range target.Metrics {
		// Compare top-level value.
		if oldVal, ok := baseMetrics[m.Name]; ok && !seen[m.Name] {
			diff.Changes = append(diff.Changes, c.metricChange(m.Name, oldVal, m.Value))
			seen[m.Name] = true
		}
		// Compare per-key values.
		for k, v := range m.Values {
			if seen[k] {
				continue
			}
			if oldVal, ok := baseMetrics[k]; ok {
				diff.Changes = append(diff.Changes, c.metricChange(k, oldVal, v))
				seen[k] = true
			}
		}
	}

	// Compare per-sample correctness.
	baseSamples := make(map[string]bool, len(base.Samples))
	for _, s := range base.Samples {
		baseSamples[s.ID] = s.Correct
	}

	for _, s := range target.Samples {
		baseCorrect, ok := baseSamples[s.ID]
		if !ok {
			continue
		}
		if !baseCorrect && s.Correct {
			diff.Fixed = append(diff.Fixed, s.ID)
		} else if baseCorrect && !s.Correct {
			diff.Regressed = append(diff.Regressed, s.ID)
		}
	}

	return diff
}

func (c *RunComparator) metricChange(name string, oldVal, newVal float64) MetricChange {
	delta := newVal - oldVal
	return MetricChange{
		Name:        name,
		OldValue:    oldVal,
		NewValue:    newVal,
		Delta:       delta,
		Improved:    delta > 0,
		Significant: math.Abs(delta) >= c.threshold,
	}
}

// Summary returns a human-readable summary of the comparison.
func (d *RunDiff) Summary() string {
	var b strings.Builder

	for _, ch := range d.Changes {
		icon := "✅"
		if ch.Delta < 0 {
			icon = "⚠️ "
		}
		sign := "+"
		if ch.Delta < 0 {
			sign = ""
		}
		fmt.Fprintf(&b, "%s %s: %.4f → %.4f (%s%.4f)\n", icon, ch.Name, ch.OldValue, ch.NewValue, sign, ch.Delta)
	}

	if len(d.Fixed) > 0 || len(d.Regressed) > 0 {
		fmt.Fprintf(&b, "Fixed: %d samples | Regressed: %d samples\n", len(d.Fixed), len(d.Regressed))
	}

	return b.String()
}

// HasRegression returns true if any metric decreased significantly.
func (d *RunDiff) HasRegression() bool {
	for _, ch := range d.Changes {
		if ch.Significant && !ch.Improved {
			return true
		}
	}
	return false
}
