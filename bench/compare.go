package bench

import (
	"fmt"
	"math"
	"strings"
)

func metricValueKey(metricName, valueName string) string {
	return metricName + "\x1f" + valueName
}

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
	// Incompatible lists judge metrics whose two runs were scored by different
	// backend judges under the same metric name — the same requested model/prompt
	// resolved to different backend models — so their scores are not directly
	// comparable even though the metric names match. The metric changes are still
	// reported, but a caller should treat a flagged metric's delta as suspect.
	Incompatible []JudgeIncompatibility
}

// JudgeIncompatibility records a judge metric that both runs computed but under
// different resolved backend models, so joining the two by metric name alone
// would silently compare scores produced by different judges.
type JudgeIncompatibility struct {
	// Metric is the judge metric name shared by both runs.
	Metric string
	// BaseResolvedModel is the backend model the base run resolved to.
	BaseResolvedModel string
	// TargetResolvedModel is the backend model the target run resolved to.
	TargetResolvedModel string
}

// MetricChange represents a change in a metric between two runs.
type MetricChange struct {
	Name        string
	OldValue    float64
	NewValue    float64
	Delta       float64
	Improved    bool
	Neutral     bool
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
			baseMetrics[metricValueKey(m.Name, k)] = v
		}
	}

	seen := make(map[string]bool)
	for _, m := range target.Metrics {
		neutral := m.Descriptive
		// Compare top-level value.
		if oldVal, ok := baseMetrics[m.Name]; ok && !seen[m.Name] {
			diff.Changes = append(diff.Changes, c.metricChange(m.Name, oldVal, m.Value, neutral))
			seen[m.Name] = true
		}
		// Compare per-key values.
		for k, v := range m.Values {
			lookupKey := metricValueKey(m.Name, k)
			if seen[lookupKey] {
				continue
			}
			if oldVal, ok := baseMetrics[lookupKey]; ok {
				diff.Changes = append(diff.Changes, c.metricChange(m.Name+"."+k, oldVal, v, neutral))
				seen[lookupKey] = true
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

	// Flag judge metrics whose two runs resolved to different backend models: the
	// metric names match but the scores were produced by different judges, so the
	// delta above is not a like-for-like comparison.
	diff.Incompatible = judgeIncompatibilities(base.Provenance.Judges, target.Provenance.Judges)

	return diff
}

// judgeIncompatibilities returns the judge metrics computed by both runs that
// resolved to different backend models. A judge metric name fixes the requested
// model and prompt, but a provider may resolve the same alias to different
// backend models across runs; comparing those scores by name alone would be
// silently wrong, so each such metric is flagged.
func judgeIncompatibilities(base, target []JudgeProvenance) []JudgeIncompatibility {
	if len(base) == 0 || len(target) == 0 {
		return nil
	}
	byMetric := make(map[string]JudgeProvenance, len(base))
	for _, j := range base {
		byMetric[j.Metric] = j
	}
	var out []JudgeIncompatibility
	for _, tj := range target {
		bj, ok := byMetric[tj.Metric]
		if !ok {
			continue
		}
		if bj.ResolvedModel != tj.ResolvedModel {
			out = append(out, JudgeIncompatibility{
				Metric:              tj.Metric,
				BaseResolvedModel:   bj.ResolvedModel,
				TargetResolvedModel: tj.ResolvedModel,
			})
		}
	}
	return out
}

func (c *RunComparator) metricChange(name string, oldVal, newVal float64, neutral bool) MetricChange {
	delta := newVal - oldVal
	improved := delta > 0
	if neutral {
		improved = false
	}
	return MetricChange{
		Name:        name,
		OldValue:    oldVal,
		NewValue:    newVal,
		Delta:       delta,
		Improved:    improved,
		Neutral:     neutral,
		Significant: math.Abs(delta) >= c.threshold,
	}
}

// Summary returns a human-readable summary of the comparison.
func (d *RunDiff) Summary() string {
	var b strings.Builder

	for _, ch := range d.Changes {
		icon := "✅"
		if ch.Neutral {
			icon = "ℹ️ "
		} else if ch.Delta < 0 {
			icon = "⚠️ "
		}
		sign := "+"
		if ch.Delta < 0 || ch.Neutral {
			sign = ""
		}
		fmt.Fprintf(&b, "%s %s: %.4f → %.4f (%s%.4f)\n", icon, ch.Name, ch.OldValue, ch.NewValue, sign, ch.Delta)
	}

	if len(d.Fixed) > 0 || len(d.Regressed) > 0 {
		fmt.Fprintf(&b, "Fixed: %d samples | Regressed: %d samples\n", len(d.Fixed), len(d.Regressed))
	}

	for _, inc := range d.Incompatible {
		fmt.Fprintf(&b, "⚠️  %s: judges differ (base %q → target %q); scores are not directly comparable\n",
			inc.Metric, inc.BaseResolvedModel, inc.TargetResolvedModel)
	}

	return b.String()
}

// HasRegression returns true if any metric decreased significantly. Judge metrics
// flagged as incompatible — the two runs resolved the same requested model/prompt
// to different backend judges — are excluded (along with their per-key subvalues),
// so a non-like-for-like judge delta is never treated as a real regression by an
// automated gate. Their deltas are still reported in [RunDiff.Changes] for display.
func (d *RunDiff) HasRegression() bool {
	incompatible := make(map[string]struct{}, len(d.Incompatible))
	for _, inc := range d.Incompatible {
		incompatible[inc.Metric] = struct{}{}
	}
	for _, ch := range d.Changes {
		if ch.Significant && !ch.Improved && !ch.Neutral && !isIncompatibleChange(ch.Name, incompatible) {
			return true
		}
	}
	return false
}

// isIncompatibleChange reports whether a metric change name belongs to an
// incompatible judge metric — either the metric's top-level value (an exact name
// match) or one of its per-key subvalues (name "<metric>.<key>"). The judge
// metric name itself contains dots (for example the ":t0.5" threshold), so the
// match is anchored on the full metric name rather than split on the first dot.
func isIncompatibleChange(name string, incompatible map[string]struct{}) bool {
	if _, ok := incompatible[name]; ok {
		return true
	}
	for metric := range incompatible {
		if strings.HasPrefix(name, metric+".") {
			return true
		}
	}
	return false
}
