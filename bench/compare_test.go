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
