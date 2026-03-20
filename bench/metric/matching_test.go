package metric

import (
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
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

	m := FuzzyMatch(0.8)
	r := m.Compute(scored)

	if r.Name != "fuzzy_match" {
		t.Errorf("Name = %q, want %q", r.Name, "fuzzy_match")
	}
	assertMatchClose(t, "FuzzyMatch (exact)", r.Value, 1.0)
	assertMatchClose(t, "mean_similarity", r.Values["mean_similarity"], 1.0)
}

func TestFuzzyMatchSimilar(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "kitten"}, Prediction: bench.Prediction[string]{Label: "sitting"}},
	}

	m := FuzzyMatch(0.5)
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

	m := FuzzyMatch(0.95)
	r := m.Compute(scored)

	// "hello" vs "helo": distance=1, maxLen=5, similarity=0.8 < 0.95
	assertMatchClose(t, "FuzzyMatch (high threshold)", r.Value, 0.0)
}

func TestFuzzyMatchEmpty(t *testing.T) {
	t.Parallel()

	m := FuzzyMatch(0.5)
	r := m.Compute(nil)
	assertMatchClose(t, "FuzzyMatch (empty)", r.Value, 0.0)
}

func TestFuzzyMatchEmptyStrings(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: ""}, Prediction: bench.Prediction[string]{Label: ""}},
	}

	m := FuzzyMatch(0.5)
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
