package metric

import (
	"errors"
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestExactMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scored []bench.ScoredSample[string]
		want   float64
	}{
		{
			name: "all correct",
			scored: []bench.ScoredSample[string]{
				{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a"}},
				{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "b"}},
			},
			want: 1.0,
		},
		{
			name: "none correct",
			scored: []bench.ScoredSample[string]{
				{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "b"}},
				{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "a"}},
			},
			want: 0.0,
		},
		{
			name: "half correct",
			scored: []bench.ScoredSample[string]{
				{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a"}},
				{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "a"}},
			},
			want: 0.5,
		},
		{
			name:   "empty",
			scored: nil,
			want:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := ExactMatch[string]()
			r := m.Compute(tt.scored)
			if r.Name != "exact_match" {
				t.Errorf("Name = %q, want %q", r.Name, "exact_match")
			}
			assertMatchClose(t, "ExactMatch", r.Value, tt.want)
		})
	}
}

func TestFuzzyMatchExact(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "hello"}, Prediction: bench.Prediction[string]{Label: "hello"}},
		{Sample: bench.Sample[string]{Label: "world"}, Prediction: bench.Prediction[string]{Label: "world"}},
	}

	m := mustFuzzyMatch(t, 0.8)
	r := m.Compute(scored)

	if r.Name != "fuzzy_match[t0.8]" {
		t.Errorf("Name = %q, want %q", r.Name, "fuzzy_match[t0.8]")
	}
	assertMatchClose(t, "FuzzyMatch (exact)", r.Value, 1.0)
	assertMatchClose(t, "mean_similarity", r.Values["mean_similarity"], 1.0)

	// The threshold is a configuration input, not a quality signal: it belongs in
	// provenance detail, not in Values where the comparator would score its change.
	if _, ok := r.Values["threshold"]; ok {
		t.Error("threshold must not appear in Values: it is a configuration input, not a quality signal")
	}
	detail, ok := r.Detail.(map[string]any)
	if !ok {
		t.Fatalf("Detail type = %T, want map[string]any", r.Detail)
	}
	if detail["threshold"] != 0.8 {
		t.Errorf("Detail[threshold] = %v, want 0.8", detail["threshold"])
	}
}

func TestFuzzyMatchSimilar(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "kitten"}, Prediction: bench.Prediction[string]{Label: "sitting"}},
	}

	m := mustFuzzyMatch(t, 0.5)
	r := m.Compute(scored)

	// "kitten" vs "sitting" → Levenshtein distance = 3, maxLen = 7, similarity ≈ 0.571
	if r.Values["mean_similarity"] < 0.4 || r.Values["mean_similarity"] > 0.7 {
		t.Errorf("mean_similarity = %f, expected ~0.57", r.Values["mean_similarity"])
	}
	// With threshold 0.5, this should count as a match.
	assertMatchClose(t, "FuzzyMatch (similar)", r.Value, 1.0)
}

func TestFuzzyMatchHighThreshold(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "hello"}, Prediction: bench.Prediction[string]{Label: "helo"}},
	}

	m := mustFuzzyMatch(t, 0.95)
	r := m.Compute(scored)

	// "hello" vs "helo": distance=1, maxLen=5, similarity=0.8 < 0.95
	assertMatchClose(t, "FuzzyMatch (high threshold)", r.Value, 0.0)
}

func TestFuzzyMatchEmpty(t *testing.T) {
	t.Parallel()

	m := mustFuzzyMatch(t, 0.5)
	r := m.Compute(nil)
	assertMatchClose(t, "FuzzyMatch (empty)", r.Value, 0.0)

	// The empty path must keep the threshold-bearing name and threshold provenance,
	// so an empty run stays distinct by cutoff and never drops the configuration
	// input the comparator relies on.
	if r.Name != "fuzzy_match[t0.5]" {
		t.Errorf("empty Name = %q, want %q", r.Name, "fuzzy_match[t0.5]")
	}
	detail, ok := r.Detail.(map[string]any)
	if !ok {
		t.Fatalf("empty Detail type = %T, want map[string]any", r.Detail)
	}
	if detail["threshold"] != 0.5 {
		t.Errorf("empty Detail[threshold] = %v, want 0.5", detail["threshold"])
	}
}

func TestFuzzyMatchEmptyStrings(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: ""}, Prediction: bench.Prediction[string]{Label: ""}},
	}

	m := mustFuzzyMatch(t, 0.5)
	r := m.Compute(scored)

	// Both empty → similarity = 1.0
	assertMatchClose(t, "FuzzyMatch (empty strings)", r.Value, 1.0)
}

func assertMatchClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

func mustFuzzyMatch(t *testing.T, threshold float64) Metric[string] {
	t.Helper()
	m, err := FuzzyMatch(threshold)
	if err != nil {
		t.Fatalf("FuzzyMatch: unexpected error: %v", err)
	}
	return m
}

// Runs scored at different thresholds must fold distinct identities so the
// comparator never joins the match rate at one cutoff against another.
func TestFuzzyMatchThresholdIdentity(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "hello"}, Prediction: bench.Prediction[string]{Label: "helo"}},
	}
	lo := mustFuzzyMatch(t, 0.5)
	hi := mustFuzzyMatch(t, 0.9)

	if lo.Name() != "fuzzy_match[t0.5]" {
		t.Errorf("low Name = %q, want %q", lo.Name(), "fuzzy_match[t0.5]")
	}
	if hi.Name() != "fuzzy_match[t0.9]" {
		t.Errorf("high Name = %q, want %q", hi.Name(), "fuzzy_match[t0.9]")
	}
	if lo.Compute(scored).Name == hi.Compute(scored).Name {
		t.Error("runs at different thresholds must not share a Result.Name")
	}
}

func TestFuzzyMatchRejectsInvalidThreshold(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		threshold float64
	}{
		{"nan", math.NaN()},
		{"pos_inf", math.Inf(1)},
		{"neg_inf", math.Inf(-1)},
		{"below_zero", -0.1},
		{"above_one", 1.1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := FuzzyMatch(tc.threshold)
			if err == nil {
				t.Fatalf("threshold %v: expected error, got nil", tc.threshold)
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeInvalidInput {
				t.Errorf("error = %v, want AppError %s", err, apperrors.ErrCodeInvalidInput)
			}
		})
	}
}
