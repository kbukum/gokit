package metric

import (
	"math"
	"strings"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func TestWeightedEqualWeights(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.2}},
	}

	p := PrecisionAtK[string]("pos", 2)
	r := RecallAtK[string]("pos", 2)

	w := Weighted[string](map[Metric[string]]float64{
		p: 0.5,
		r: 0.5,
	})

	result := w.Compute(scored)

	// Name should contain "weighted("
	if !strings.HasPrefix(result.Name, "weighted(") {
		t.Errorf("Name = %q, want prefix %q", result.Name, "weighted(")
	}

	// PrecisionAtK@2 = 2/2 = 1.0 (both top are pos)
	// RecallAtK@2 = 2/2 = 1.0 (both pos found)
	// Weighted = 0.5*1.0 + 0.5*1.0 = 1.0
	assertCompositeClose(t, "weighted (equal)", result.Value, 1.0)
}

func TestWeightedDifferentWeights(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.6}},
	}

	p := PrecisionAtK[string]("pos", 2)
	r := RecallAtK[string]("pos", 2)

	w := Weighted[string](map[Metric[string]]float64{
		p: 0.7,
		r: 0.3,
	})

	result := w.Compute(scored)

	// Precision@2 = 1/2 = 0.5 (top2: pos, neg)
	// Recall@2 = 1/2 = 0.5 (1 of 2 pos found)
	// Weighted = 0.7*0.5 + 0.3*0.5 = 0.35 + 0.15 = 0.5
	assertCompositeClose(t, "weighted (diff)", result.Value, 0.5)
}

func TestWeightedValuesContainIndividual(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
	}

	p := PrecisionAtK[string]("pos", 1)
	r := RecallAtK[string]("pos", 1)

	w := Weighted[string](map[Metric[string]]float64{
		p: 0.5,
		r: 0.5,
	})

	result := w.Compute(scored)

	if result.Values == nil {
		t.Fatal("Values is nil")
	}

	pName := p.Name()
	rName := r.Name()

	if _, ok := result.Values[pName]; !ok {
		t.Errorf("Values missing key %q", pName)
	}
	if _, ok := result.Values[rName]; !ok {
		t.Errorf("Values missing key %q", rName)
	}
}

func TestWeightedDetail(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.9}},
	}

	p := PrecisionAtK[string]("pos", 1)

	w := Weighted[string](map[Metric[string]]float64{
		p: 1.0,
	})

	result := w.Compute(scored)

	details, ok := result.Detail.([]Result)
	if !ok {
		t.Fatalf("Detail type = %T, want []Result", result.Detail)
	}
	if len(details) != 1 {
		t.Errorf("len(Detail) = %d, want 1", len(details))
	}
}

func TestWeightedEmpty(t *testing.T) {
	t.Parallel()

	p := PrecisionAtK[string]("pos", 1)
	w := Weighted[string](map[Metric[string]]float64{
		p: 1.0,
	})

	result := w.Compute(nil)
	assertCompositeClose(t, "weighted (empty)", result.Value, 0)
}

func TestWeightedDirection(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.1}},
		{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 1.7}},
	}

	// Two lower-is-better error metrics with positive weights → composite is
	// lower-is-better, and each component keeps its own direction.
	lowerOnly := Weighted[float64](map[Metric[float64]]float64{
		MAE(): 0.5,
		MSE(): 0.5,
	}).Compute(scored)
	if lowerOnly.Direction != bench.LowerIsBetter {
		t.Errorf("lower+lower Direction = %v, want LowerIsBetter", lowerOnly.Direction)
	}
	if got := lowerOnly.Directions["mae"]; got != bench.LowerIsBetter {
		t.Errorf("component mae Direction = %v, want LowerIsBetter", got)
	}

	// A negative weight flips a lower-is-better component to a higher-is-better
	// contribution; mixed with a positive lower-is-better term the sum has no
	// single direction → neutral.
	mixed := Weighted[float64](map[Metric[float64]]float64{
		MAE(): -0.5,
		MSE(): 0.5,
	}).Compute(scored)
	if mixed.Direction != bench.Neutral {
		t.Errorf("negative-weight mix Direction = %v, want Neutral", mixed.Direction)
	}

	// Two lower-is-better metrics, one with a negative weight, still disagree →
	// but two negatives would agree. Both negative → higher-is-better sum.
	bothNeg := Weighted[float64](map[Metric[float64]]float64{
		MAE(): -0.5,
		MSE(): -0.5,
	}).Compute(scored)
	if bothNeg.Direction != bench.HigherIsBetter {
		t.Errorf("both-negative lower-is-better Direction = %v, want HigherIsBetter", bothNeg.Direction)
	}
}

func assertCompositeClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}
