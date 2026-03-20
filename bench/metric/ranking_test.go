package metric

import (
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func assertRankClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

// --- NDCG ---

func TestNDCGPerfectRanking(t *testing.T) {
	t.Parallel()

	// All predicted labels match actual labels, sorted by descending score.
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "b", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "c"}, Prediction: bench.Prediction[string]{Label: "c", Score: 0.7}},
	}

	m := NDCG[string](0)
	r := m.Compute(scored)

	if r.Name != "ndcg" {
		t.Errorf("Name = %q, want %q", r.Name, "ndcg")
	}
	assertRankClose(t, "ndcg (perfect)", r.Value, 1.0)
}

func TestNDCGReversedRanking(t *testing.T) {
	t.Parallel()

	// Top-scored items have wrong labels, low-scored items have correct labels.
	// Scores: 0.9 (wrong), 0.8 (wrong), 0.7 (correct), 0.6 (correct)
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "c"}, Prediction: bench.Prediction[string]{Label: "c", Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "d"}, Prediction: bench.Prediction[string]{Label: "d", Score: 0.6}},
	}

	m := NDCG[string](0)
	r := m.Compute(scored)

	if r.Value >= 1.0 {
		t.Errorf("ndcg = %f, expected < 1.0 for non-ideal ranking", r.Value)
	}
	if r.Value <= 0 {
		t.Errorf("ndcg = %f, expected > 0", r.Value)
	}
}

func TestNDCGWithKLimit(t *testing.T) {
	t.Parallel()

	// Only the top-1 item is correct.
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "c"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.7}},
	}

	m := NDCG[string](1)
	r := m.Compute(scored)

	if r.Name != "ndcg@1" {
		t.Errorf("Name = %q, want %q", r.Name, "ndcg@1")
	}
	// Top-1 is correct → NDCG@1 = 1.0
	assertRankClose(t, "ndcg@1", r.Value, 1.0)
}

func TestNDCGEmpty(t *testing.T) {
	t.Parallel()

	m := NDCG[string](0)
	r := m.Compute(nil)
	assertRankClose(t, "ndcg (empty)", r.Value, 0)
}

func TestNDCGAllWrong(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "y", Score: 0.8}},
	}

	m := NDCG[string](0)
	r := m.Compute(scored)

	// No relevant items → DCG=0, idealDCG=0, safeDivide returns 0.
	assertRankClose(t, "ndcg (all wrong)", r.Value, 0)
}

func TestNDCGValuesContainDCG(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.5}},
	}

	m := NDCG[string](0)
	r := m.Compute(scored)

	if _, ok := r.Values["dcg"]; !ok {
		t.Error("missing dcg in Values")
	}
	if _, ok := r.Values["ideal_dcg"]; !ok {
		t.Error("missing ideal_dcg in Values")
	}
}

// --- MAP ---

func TestMAPAllRelevantAtTop(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.2}},
	}

	m := MAP[string]("pos")
	r := m.Compute(scored)

	if r.Name != "map" {
		t.Errorf("Name = %q, want %q", r.Name, "map")
	}
	// Both relevant at rank 1 and 2 → AP = (1/1 + 2/2) / 2 = 1.0
	assertRankClose(t, "map (top)", r.Value, 1.0)
}

func TestMAPMixedRelevance(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.6}},
	}

	m := MAP[string]("pos")
	r := m.Compute(scored)

	// Sorted by score desc: neg(0.9), pos(0.8), neg(0.7), pos(0.6)
	// pos at rank 2: precision=1/2=0.5
	// pos at rank 4: precision=2/4=0.5
	// MAP = (0.5 + 0.5) / 2 = 0.5
	assertRankClose(t, "map (mixed)", r.Value, 0.5)
}

func TestMAPNoRelevant(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
	}

	m := MAP[string]("pos")
	r := m.Compute(scored)
	assertRankClose(t, "map (no relevant)", r.Value, 0)
}

func TestMAPEmpty(t *testing.T) {
	t.Parallel()

	m := MAP[string]("pos")
	r := m.Compute(nil)
	assertRankClose(t, "map (empty)", r.Value, 0)
}

// --- PrecisionAtK ---

func TestPrecisionAtKAllRelevant(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
	}

	m := PrecisionAtK[string]("pos", 2)
	r := m.Compute(scored)

	if r.Name != "precision@2" {
		t.Errorf("Name = %q, want %q", r.Name, "precision@2")
	}
	assertRankClose(t, "precision@2 (all relevant)", r.Value, 1.0)
}

func TestPrecisionAtKNoneRelevant(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.3}},
	}

	m := PrecisionAtK[string]("pos", 2)
	r := m.Compute(scored)
	assertRankClose(t, "precision@2 (none)", r.Value, 0)
}

func TestPrecisionAtKMixed(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.7}},
	}

	m := PrecisionAtK[string]("pos", 2)
	r := m.Compute(scored)
	// Top 2: pos, neg → 1/2 = 0.5
	assertRankClose(t, "precision@2 (mixed)", r.Value, 0.5)
}

func TestPrecisionAtKEmpty(t *testing.T) {
	t.Parallel()

	m := PrecisionAtK[string]("pos", 3)
	r := m.Compute(nil)
	assertRankClose(t, "precision@3 (empty)", r.Value, 0)
}

func TestPrecisionAtKZeroK(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
	}

	m := PrecisionAtK[string]("pos", 0)
	r := m.Compute(scored)
	assertRankClose(t, "precision@0", r.Value, 0)
}

func TestPrecisionAtKExceedsLength(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
	}

	m := PrecisionAtK[string]("pos", 10)
	r := m.Compute(scored)
	// k=10 but only 1 sample, and it's relevant → 1/1 = 1.0
	assertRankClose(t, "precision@10 (exceeds)", r.Value, 1.0)
}

// --- RecallAtK ---

func TestRecallAtKAllFound(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
	}

	m := RecallAtK[string]("pos", 2)
	r := m.Compute(scored)

	if r.Name != "recall@2" {
		t.Errorf("Name = %q, want %q", r.Name, "recall@2")
	}
	// Both positives in top 2 → recall = 2/2 = 1.0
	assertRankClose(t, "recall@2 (all found)", r.Value, 1.0)
}

func TestRecallAtKPartial(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.7}},
	}

	m := RecallAtK[string]("pos", 1)
	r := m.Compute(scored)
	// Only 1 of 2 positives found in top 1 → 0.5
	assertRankClose(t, "recall@1 (partial)", r.Value, 0.5)
}

func TestRecallAtKNoneFound(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.3}},
	}

	m := RecallAtK[string]("pos", 2)
	r := m.Compute(scored)
	assertRankClose(t, "recall@2 (none)", r.Value, 0)
}

func TestRecallAtKNoRelevant(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.9}},
	}

	m := RecallAtK[string]("pos", 1)
	r := m.Compute(scored)
	assertRankClose(t, "recall@1 (no relevant)", r.Value, 0)
}

func TestRecallAtKEmpty(t *testing.T) {
	t.Parallel()

	m := RecallAtK[string]("pos", 3)
	r := m.Compute(nil)
	assertRankClose(t, "recall@3 (empty)", r.Value, 0)
}

func TestRecallAtKZeroK(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
	}

	m := RecallAtK[string]("pos", 0)
	r := m.Compute(scored)
	assertRankClose(t, "recall@0", r.Value, 0)
}

// --- NDCG known value computation ---

func TestNDCGKnownValue(t *testing.T) {
	t.Parallel()

	// 4 items: positions 0,1 correct; positions 2,3 wrong.
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "b", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "c"}, Prediction: bench.Prediction[string]{Label: "x", Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "d"}, Prediction: bench.Prediction[string]{Label: "y", Score: 0.6}},
	}

	m := NDCG[string](0)
	r := m.Compute(scored)

	// Relevances after sort by score desc: [1, 1, 0, 0]
	// DCG = 1/log2(2) + 1/log2(3) + 0 + 0 = 1.0 + 0.630930
	// Ideal relevances sorted desc: [1, 1, 0, 0] (same as above in this case)
	// NDCG = DCG/idealDCG = 1.0
	assertRankClose(t, "ndcg (top2 correct)", r.Value, 1.0)
}
