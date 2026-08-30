package metric

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func TestAUCROCPerfectClassifier(t *testing.T) {
	t.Parallel()

	// Perfect classifier: all positives have higher scores than negatives.
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.3}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.1}},
	}

	m := AUCROC[string]("pos")
	r := m.Compute(scored)

	if r.Name != "aucroc" {
		t.Errorf("Name = %q, want %q", r.Name, "aucroc")
	}
	assertProbClose(t, "AUC (perfect)", r.Value, 1.0)
}

func TestAUCROCRandomClassifier(t *testing.T) {
	t.Parallel()

	// Interleaved scores → AUC should be around 0.5.
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.6}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.5}},
	}

	m := AUCROC[string]("pos")
	r := m.Compute(scored)

	// AUC should be 0.75 for this specific ordering
	if r.Value < 0.0 || r.Value > 1.0 {
		t.Errorf("AUC = %f, expected in [0, 1]", r.Value)
	}
}

func TestAUCROCEmpty(t *testing.T) {
	t.Parallel()

	m := AUCROC[string]("pos")
	r := m.Compute(nil)
	assertProbClose(t, "AUC (empty)", r.Value, 0)
}

func TestAUCROCAllSameClass(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.8}},
	}

	m := AUCROC[string]("pos")
	r := m.Compute(scored)
	// With only positives (no negatives), AUC is 0.
	assertProbClose(t, "AUC (all same)", r.Value, 0)
}

func TestAUCROCHasROCCurve(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.1}},
	}

	m := AUCROC[string]("pos")
	r := m.Compute(scored)

	roc, ok := r.Detail.(bench.ROCCurve)
	if !ok {
		t.Fatalf("Detail is not ROCCurve, got %T", r.Detail)
	}
	if len(roc.FPR) == 0 {
		t.Error("ROCCurve.FPR is empty")
	}
	if len(roc.TPR) == 0 {
		t.Error("ROCCurve.TPR is empty")
	}
}

func TestBrierScorePerfect(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 1.0}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.0}},
	}

	m := BrierScore[string]("pos")
	r := m.Compute(scored)

	if r.Name != "brier_score" {
		t.Errorf("Name = %q, want %q", r.Name, "brier_score")
	}
	assertProbClose(t, "Brier (perfect)", r.Value, 0.0)
}

func TestBrierScoreWorst(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.0}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 1.0}},
	}

	m := BrierScore[string]("pos")
	r := m.Compute(scored)

	assertProbClose(t, "Brier (worst)", r.Value, 1.0)
}

func TestBrierScoreEmpty(t *testing.T) {
	t.Parallel()

	m := BrierScore[string]("pos")
	r := m.Compute(nil)
	assertProbClose(t, "Brier (empty)", r.Value, 0)
}

func TestLogLossPerfect(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.999}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.001}},
	}

	m := LogLoss[string]("pos")
	r := m.Compute(scored)

	if r.Name != "log_loss" {
		t.Errorf("Name = %q, want %q", r.Name, "log_loss")
	}
	// Near-perfect predictions → log loss near 0.
	if r.Value > 0.01 {
		t.Errorf("LogLoss (near-perfect) = %f, want < 0.01", r.Value)
	}
}

func TestLogLossWorst(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.001}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.999}},
	}

	m := LogLoss[string]("pos")
	r := m.Compute(scored)

	// Worst case → high log loss.
	if r.Value < 1.0 {
		t.Errorf("LogLoss (worst) = %f, want > 1.0", r.Value)
	}
}

func TestLogLossEmpty(t *testing.T) {
	t.Parallel()

	m := LogLoss[string]("pos")
	r := m.Compute(nil)
	assertProbClose(t, "LogLoss (empty)", r.Value, 0)
}

func assertProbClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-4 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

// TestProbabilityMetricDirections asserts each probability metric declares the
// optimization direction that run comparison classifies against: aucroc is
// higher-is-better, brier_score/log_loss/calibration are lower-is-better.
func TestProbabilityMetricDirections(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.1}},
	}

	tests := []struct {
		name   string
		metric Metric[string]
		want   bench.Direction
	}{
		{"aucroc", AUCROC[string]("pos"), bench.HigherIsBetter},
		{"brier_score", BrierScore[string]("pos"), bench.LowerIsBetter},
		{"log_loss", LogLoss[string]("pos"), bench.LowerIsBetter},
		{"calibration", Calibration[string]("pos", 10), bench.LowerIsBetter},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.metric.Compute(scored).Direction; got != tc.want {
				t.Errorf("%s Direction = %v, want %v", tc.name, got, tc.want)
			}
			if got := tc.metric.Compute(nil).Direction; got != tc.want {
				t.Errorf("%s empty Direction = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestMetricResultSerializesDirection asserts a lower-is-better metric result
// serializes direction as its snake_case string and emits no legacy
// descriptive field.
func TestMetricResultSerializesDirection(t *testing.T) {
	t.Parallel()

	res := MAE().Compute([]bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 0.5}},
	})

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got := m["direction"]; got != "lower_is_better" {
		t.Errorf("direction = %v, want %q", got, "lower_is_better")
	}
	if _, ok := m["descriptive"]; ok {
		t.Errorf("descriptive present in %s, want it removed", data)
	}
}
