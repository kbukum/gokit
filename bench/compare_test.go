package bench

import (
	"strings"
	"testing"
	"time"
)

func TestRunComparatorImprovement(t *testing.T) {
	t.Parallel()

	base := &RunResult{
		ID:        "base-run",
		Timestamp: time.Now(),
		Metrics: []MetricResult{
			{Name: "f1", Value: 0.80},
			{Name: "accuracy", Value: 0.75},
		},
		Samples: []SampleResult{
			{ID: "s1", Correct: false},
			{ID: "s2", Correct: true},
		},
	}
	target := &RunResult{
		ID:        "target-run",
		Timestamp: time.Now(),
		Metrics: []MetricResult{
			{Name: "f1", Value: 0.90},
			{Name: "accuracy", Value: 0.85},
		},
		Samples: []SampleResult{
			{ID: "s1", Correct: true}, // fixed
			{ID: "s2", Correct: true},
		},
	}

	cmp := NewRunComparator()
	diff := cmp.Compare(base, target)

	if diff.BaseID != "base-run" {
		t.Errorf("BaseID = %q, want %q", diff.BaseID, "base-run")
	}
	if diff.TargetID != "target-run" {
		t.Errorf("TargetID = %q, want %q", diff.TargetID, "target-run")
	}

	// All metrics improved.
	for _, ch := range diff.Changes {
		if !ch.Improved {
			t.Errorf("metric %q should be improved (delta = %f)", ch.Name, ch.Delta)
		}
	}

	// s1 was fixed.
	if len(diff.Fixed) != 1 || diff.Fixed[0] != "s1" {
		t.Errorf("Fixed = %v, want [s1]", diff.Fixed)
	}
	if len(diff.Regressed) != 0 {
		t.Errorf("Regressed = %v, want []", diff.Regressed)
	}
	if diff.HasRegression() {
		t.Error("HasRegression() = true, want false")
	}
}

func TestRunComparatorDoesNotJoinDifferentThresholdIdentities(t *testing.T) {
	t.Parallel()

	// A threshold-dependent metric folds its cutoff into the name, so two runs
	// scored at different thresholds carry distinct names and must never be joined
	// as comparable: the F1/match rate at one cutoff is not the same quantity at
	// another, so reporting a delta between them would be an incomparable diff.
	base := &RunResult{
		ID: "base",
		Metrics: []MetricResult{
			{Name: "classification[t0.3]", Value: 0.90},
			{Name: "fuzzy_match[t0.5]", Value: 0.80},
		},
	}
	target := &RunResult{
		ID: "target",
		Metrics: []MetricResult{
			{Name: "classification[t0.7]", Value: 0.60},
			{Name: "fuzzy_match[t0.9]", Value: 0.40},
		},
	}

	diff := NewRunComparator().Compare(base, target)

	if len(diff.Changes) != 0 {
		t.Errorf("Changes = %v, want none: differently-thresholded metrics must not join", diff.Changes)
	}
}

func TestRunComparatorRegression(t *testing.T) {
	t.Parallel()

	base := &RunResult{
		ID: "base",
		Metrics: []MetricResult{
			{Name: "f1", Value: 0.90},
		},
		Samples: []SampleResult{
			{ID: "s1", Correct: true},
			{ID: "s2", Correct: true},
		},
	}
	target := &RunResult{
		ID: "target",
		Metrics: []MetricResult{
			{Name: "f1", Value: 0.70},
		},
		Samples: []SampleResult{
			{ID: "s1", Correct: true},
			{ID: "s2", Correct: false}, // regressed
		},
	}

	cmp := NewRunComparator()
	diff := cmp.Compare(base, target)

	if !diff.HasRegression() {
		t.Error("HasRegression() = false, want true")
	}
	if len(diff.Regressed) != 1 || diff.Regressed[0] != "s2" {
		t.Errorf("Regressed = %v, want [s2]", diff.Regressed)
	}
}

func TestRunComparatorThreshold(t *testing.T) {
	t.Parallel()

	base := &RunResult{
		ID:      "base",
		Metrics: []MetricResult{{Name: "f1", Value: 0.80}},
	}
	target := &RunResult{
		ID:      "target",
		Metrics: []MetricResult{{Name: "f1", Value: 0.795}}, // only 0.005 change
	}

	cmp := NewRunComparator(WithChangeThreshold(0.01))
	diff := cmp.Compare(base, target)

	for _, ch := range diff.Changes {
		if ch.Significant {
			t.Errorf("change of %f should not be significant with threshold 0.01", ch.Delta)
		}
	}

	if diff.HasRegression() {
		t.Error("HasRegression() = true, want false (below threshold)")
	}
}

func TestRunComparatorWithSubValues(t *testing.T) {
	t.Parallel()

	base := &RunResult{
		ID: "base",
		Metrics: []MetricResult{
			{
				Name:  "classification",
				Value: 0.80,
				Values: map[string]float64{
					"precision": 0.85,
					"recall":    0.75,
				},
			},
		},
	}
	target := &RunResult{
		ID: "target",
		Metrics: []MetricResult{
			{
				Name:  "classification",
				Value: 0.90,
				Values: map[string]float64{
					"precision": 0.90,
					"recall":    0.90,
				},
			},
		},
	}

	cmp := NewRunComparator()
	diff := cmp.Compare(base, target)

	if len(diff.Changes) < 2 {
		t.Fatalf("expected at least 2 changes, got %d", len(diff.Changes))
	}
}

func TestRunDiffSummary(t *testing.T) {
	t.Parallel()

	diff := &RunDiff{
		BaseID:   "base",
		TargetID: "target",
		Changes: []MetricChange{
			{Name: "f1", OldValue: 0.80, NewValue: 0.90, Delta: 0.10, Improved: true, Significant: true},
			{Name: "accuracy", OldValue: 0.85, NewValue: 0.80, Delta: -0.05, Improved: false, Significant: true},
		},
		Fixed:     []string{"s1"},
		Regressed: []string{"s2"},
	}

	summary := diff.Summary()
	if summary == "" {
		t.Fatal("Summary() returned empty string")
	}
	if !strings.Contains(summary, "f1") {
		t.Error("Summary should contain metric name 'f1'")
	}
	if !strings.Contains(summary, "Fixed: 1") {
		t.Error("Summary should mention Fixed: 1")
	}
	if !strings.Contains(summary, "Regressed: 1") {
		t.Error("Summary should mention Regressed: 1")
	}
}

func TestRunDiffHasRegressionNoChanges(t *testing.T) {
	t.Parallel()

	diff := &RunDiff{}
	if diff.HasRegression() {
		t.Error("HasRegression() = true for empty diff")
	}
}

func TestRunComparatorSkipsDescriptiveMetrics(t *testing.T) {
	t.Parallel()

	// A descriptive metric whose value changes must appear in the diff for
	// visibility, but remain neutral for regression classification.
	base := &RunResult{
		ID: "base",
		Metrics: []MetricResult{
			{
				Name:        "token_stats[heuristic]",
				Value:       100,
				Descriptive: true,
				Values:      map[string]float64{"predicted_tokens_total": 100},
			},
		},
	}
	target := &RunResult{
		ID: "target",
		Metrics: []MetricResult{
			{
				Name:        "token_stats[heuristic]",
				Value:       10,
				Descriptive: true,
				Values:      map[string]float64{"predicted_tokens_total": 10},
			},
		},
	}

	diff := NewRunComparator().Compare(base, target)

	if len(diff.Changes) != 2 {
		t.Fatalf("len(Changes) = %d, want 2 (metric + subvalue)", len(diff.Changes))
	}
	for _, ch := range diff.Changes {
		if !ch.Neutral {
			t.Errorf("change %q is not neutral", ch.Name)
		}
	}
	if diff.HasRegression() {
		t.Error("HasRegression() = true, want false (descriptive metric neutral)")
	}
}

func TestRunComparatorDoesNotCrossCompareMetricSubvalues(t *testing.T) {
	t.Parallel()

	base := &RunResult{
		ID: "base",
		Metrics: []MetricResult{
			{
				Name:   "token_stats[heuristic]",
				Value:  1,
				Values: map[string]float64{"predicted_tokens_total": 100},
			},
		},
	}
	target := &RunResult{
		ID: "target",
		Metrics: []MetricResult{
			{
				Name:   "token_stats[tiktoken:cl100k_base]",
				Value:  1,
				Values: map[string]float64{"predicted_tokens_total": 120},
			},
		},
	}

	diff := NewRunComparator().Compare(base, target)
	if len(diff.Changes) != 0 {
		t.Fatalf("len(Changes) = %d, want 0 for incompatible metric identities", len(diff.Changes))
	}
}

// Two runs that computed the same judge metric name but resolved it to different
// backend models are flagged as incompatible: the metric names match, yet the
// scores were produced by different judges, so their delta is not like-for-like.
func TestRunComparatorFlagsIncompatibleJudges(t *testing.T) {
	t.Parallel()

	const judge = "llm_judge[openai/gpt-4o-mini@id@1.0.0#abc:t0.5]"
	base := &RunResult{
		ID:      "base-run",
		Metrics: []MetricResult{{Name: judge, Value: 0.80}},
		Provenance: RunProvenance{
			Judges: []JudgeProvenance{{Metric: judge, Model: "gpt-4o-mini", ResolvedModel: "gpt-4o-mini-2024-05"}},
		},
	}
	target := &RunResult{
		ID:      "target-run",
		Metrics: []MetricResult{{Name: judge, Value: 0.90}},
		Provenance: RunProvenance{
			Judges: []JudgeProvenance{{Metric: judge, Model: "gpt-4o-mini", ResolvedModel: "gpt-4o-mini-2024-11"}},
		},
	}

	diff := NewRunComparator().Compare(base, target)
	if len(diff.Incompatible) != 1 {
		t.Fatalf("Incompatible = %d entries, want 1", len(diff.Incompatible))
	}
	inc := diff.Incompatible[0]
	if inc.Metric != judge {
		t.Errorf("Incompatible[0].Metric = %q, want %q", inc.Metric, judge)
	}
	if inc.BaseResolvedModel != "gpt-4o-mini-2024-05" || inc.TargetResolvedModel != "gpt-4o-mini-2024-11" {
		t.Errorf("resolved models = %q/%q, want gpt-4o-mini-2024-05/gpt-4o-mini-2024-11",
			inc.BaseResolvedModel, inc.TargetResolvedModel)
	}
	if !strings.Contains(diff.Summary(), "judges differ") {
		t.Errorf("Summary() = %q, want it to warn that judges differ", diff.Summary())
	}
}

// A judge metric whose two runs resolved to different backend models is not a
// like-for-like comparison, so its decreased score (and per-key subvalues, whose
// names extend the dot-containing metric name) must not be classified as a
// regression by an automated gate, even though the delta is still reported.
func TestRunComparatorExcludesIncompatibleJudgeFromRegression(t *testing.T) {
	t.Parallel()

	const judge = "llm_judge[openai/gpt-4o-mini@id@1.0.0#abc:t0.5]"
	prov := func(resolved string) RunProvenance {
		return RunProvenance{
			Judges: []JudgeProvenance{{Metric: judge, Model: "gpt-4o-mini", ResolvedModel: resolved}},
		}
	}
	metrics := func(v float64) []MetricResult {
		return []MetricResult{{Name: judge, Value: v, Values: map[string]float64{"pass_rate": v}}}
	}
	base := &RunResult{ID: "base", Metrics: metrics(0.90), Provenance: prov("gpt-4o-mini-2024-05")}
	target := &RunResult{ID: "target", Metrics: metrics(0.50), Provenance: prov("gpt-4o-mini-2024-11")}

	diff := NewRunComparator().Compare(base, target)
	if len(diff.Incompatible) != 1 {
		t.Fatalf("Incompatible = %d entries, want 1", len(diff.Incompatible))
	}
	if diff.HasRegression() {
		t.Error("HasRegression() = true, want false for an incompatible judge's decreased score")
	}

	// The same decrease between judges that resolved identically is a real regression.
	same := &RunResult{ID: "same", Metrics: metrics(0.50), Provenance: prov("gpt-4o-mini-2024-05")}
	compatibleBase := &RunResult{ID: "base2", Metrics: metrics(0.90), Provenance: prov("gpt-4o-mini-2024-05")}
	if diff := NewRunComparator().Compare(compatibleBase, same); !diff.HasRegression() {
		t.Error("HasRegression() = false, want true for a like-for-like judge decrease")
	}
}

// Two runs whose judge metric resolved to the same backend model are comparable
// and raise no incompatibility flag.
func TestRunComparatorAllowsMatchingJudges(t *testing.T) {
	t.Parallel()

	const judge = "llm_judge[openai/gpt-4o-mini@id@1.0.0#abc:t0.5]"
	prov := RunProvenance{
		Judges: []JudgeProvenance{{Metric: judge, Model: "gpt-4o-mini", ResolvedModel: "gpt-4o-mini-2024-05"}},
	}
	base := &RunResult{ID: "base", Metrics: []MetricResult{{Name: judge, Value: 0.8}}, Provenance: prov}
	target := &RunResult{ID: "target", Metrics: []MetricResult{{Name: judge, Value: 0.9}}, Provenance: prov}

	if diff := NewRunComparator().Compare(base, target); len(diff.Incompatible) != 0 {
		t.Errorf("Incompatible = %v, want none for matching resolved judges", diff.Incompatible)
	}
}
