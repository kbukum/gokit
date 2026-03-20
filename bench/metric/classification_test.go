package metric

import (
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func TestBinaryClassificationKnownValues(t *testing.T) {
	t.Parallel()

	// 3 TP, 1 FP, 1 FN, 1 TN
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.8}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.7}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Label: "neg", Score: 0.8}}, // score >= 0.5 → predicted positive
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Label: "pos", Score: 0.3}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Label: "neg", Score: 0.2}},
	}
	// With threshold 0.5:
	// s0: actual=pos, score=0.9 >= 0.5 → predicted pos → TP
	// s1: actual=pos, score=0.8 >= 0.5 → predicted pos → TP
	// s2: actual=pos, score=0.7 >= 0.5 → predicted pos → TP
	// s3: actual=neg, score=0.8 >= 0.5 → predicted pos → FP
	// s4: actual=pos, score=0.3 < 0.5 → predicted neg → FN
	// s5: actual=neg, score=0.2 < 0.5 → predicted neg → TN
	// TP=3, FP=1, FN=1, TN=1
	// precision = 3/4 = 0.75
	// recall = 3/4 = 0.75
	// f1 = 0.75
	// accuracy = 4/6 ≈ 0.6667

	m := BinaryClassification[string]("pos")
	r := m.Compute(scored)

	if r.Name != "classification" {
		t.Errorf("Name = %q, want %q", r.Name, "classification")
	}

	assertClose(t, "precision", r.Values["precision"], 0.75)
	assertClose(t, "recall", r.Values["recall"], 0.75)
	assertClose(t, "f1", r.Values["f1"], 0.75)
	assertClose(t, "accuracy", r.Values["accuracy"], 4.0/6.0)
	assertClose(t, "tp", r.Values["tp"], 3)
	assertClose(t, "fp", r.Values["fp"], 1)
	assertClose(t, "tn", r.Values["tn"], 1)
	assertClose(t, "fn", r.Values["fn"], 1)
}

func TestBinaryClassificationPerfect(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.1}},
	}

	m := BinaryClassification[string]("pos")
	r := m.Compute(scored)

	assertClose(t, "precision", r.Values["precision"], 1.0)
	assertClose(t, "recall", r.Values["recall"], 1.0)
	assertClose(t, "f1", r.Values["f1"], 1.0)
	assertClose(t, "accuracy", r.Values["accuracy"], 1.0)
}

func TestBinaryClassificationCustomThreshold(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.6}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.4}},
	}

	// With threshold 0.7, score 0.6 < 0.7 → predicted neg → FN
	m := BinaryClassification[string]("pos", WithThreshold(0.7))
	r := m.Compute(scored)

	assertClose(t, "fn", r.Values["fn"], 1)
	assertClose(t, "recall", r.Values["recall"], 0)
}

func TestBinaryClassificationEmpty(t *testing.T) {
	t.Parallel()

	m := BinaryClassification[string]("pos")
	r := m.Compute(nil)

	assertClose(t, "f1", r.Value, 0)
}

func TestThresholdSweep(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.9}},
		{Sample: bench.Sample[string]{Label: "pos"}, Prediction: bench.Prediction[string]{Score: 0.6}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.4}},
		{Sample: bench.Sample[string]{Label: "neg"}, Prediction: bench.Prediction[string]{Score: 0.1}},
	}

	m := ThresholdSweep[string]("pos", []float64{0.3, 0.5, 0.7})
	r := m.Compute(scored)

	if r.Name != "threshold_sweep" {
		t.Errorf("Name = %q, want %q", r.Name, "threshold_sweep")
	}

	points, ok := r.Detail.([]bench.ThresholdPoint)
	if !ok {
		t.Fatalf("Detail is not []ThresholdPoint, got %T", r.Detail)
	}
	if len(points) != 3 {
		t.Fatalf("len(points) = %d, want 3", len(points))
	}

	// bestF1 should be > 0
	if r.Value <= 0 {
		t.Errorf("bestF1 = %f, want > 0", r.Value)
	}
}

func TestMultiClassClassification(t *testing.T) {
	t.Parallel()

	labels := []string{"cat", "dog", "bird"}
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "cat"}, Prediction: bench.Prediction[string]{Label: "cat"}},
		{Sample: bench.Sample[string]{Label: "dog"}, Prediction: bench.Prediction[string]{Label: "dog"}},
		{Sample: bench.Sample[string]{Label: "bird"}, Prediction: bench.Prediction[string]{Label: "cat"}}, // wrong
		{Sample: bench.Sample[string]{Label: "cat"}, Prediction: bench.Prediction[string]{Label: "cat"}},
	}

	m := MultiClassClassification(labels)
	r := m.Compute(scored)

	if r.Name != "multi_class_classification" {
		t.Errorf("Name = %q, want %q", r.Name, "multi_class_classification")
	}

	// accuracy = 3/4 = 0.75
	assertClose(t, "accuracy", r.Values["accuracy"], 0.75)

	// macro_f1 should be > 0
	if r.Values["macro_f1"] <= 0 {
		t.Errorf("macro_f1 = %f, want > 0", r.Values["macro_f1"])
	}
	if r.Values["micro_f1"] <= 0 {
		t.Errorf("micro_f1 = %f, want > 0", r.Values["micro_f1"])
	}
}

func TestMultiClassPerfect(t *testing.T) {
	t.Parallel()

	labels := []string{"a", "b"}
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "a"}, Prediction: bench.Prediction[string]{Label: "a"}},
		{Sample: bench.Sample[string]{Label: "b"}, Prediction: bench.Prediction[string]{Label: "b"}},
	}

	m := MultiClassClassification(labels)
	r := m.Compute(scored)

	assertClose(t, "accuracy", r.Values["accuracy"], 1.0)
	assertClose(t, "macro_f1", r.Values["macro_f1"], 1.0)
	assertClose(t, "micro_f1", r.Values["micro_f1"], 1.0)
}

func TestConfusionMatrix(t *testing.T) {
	t.Parallel()

	labels := []string{"cat", "dog"}
	scored := []bench.ScoredSample[string]{
		{Sample: bench.Sample[string]{Label: "cat"}, Prediction: bench.Prediction[string]{Label: "cat"}},
		{Sample: bench.Sample[string]{Label: "cat"}, Prediction: bench.Prediction[string]{Label: "dog"}},
		{Sample: bench.Sample[string]{Label: "dog"}, Prediction: bench.Prediction[string]{Label: "dog"}},
		{Sample: bench.Sample[string]{Label: "dog"}, Prediction: bench.Prediction[string]{Label: "dog"}},
	}

	m := ConfusionMatrix(labels)
	r := m.Compute(scored)

	if r.Name != "confusion_matrix" {
		t.Errorf("Name = %q, want %q", r.Name, "confusion_matrix")
	}

	cm, ok := r.Detail.(bench.ConfusionMatrixDetail)
	if !ok {
		t.Fatalf("Detail is not ConfusionMatrixDetail, got %T", r.Detail)
	}
	// Expected: cat→cat=1, cat→dog=1, dog→cat=0, dog→dog=2
	if cm.Matrix[0][0] != 1 {
		t.Errorf("matrix[cat][cat] = %d, want 1", cm.Matrix[0][0])
	}
	if cm.Matrix[0][1] != 1 {
		t.Errorf("matrix[cat][dog] = %d, want 1", cm.Matrix[0][1])
	}
	if cm.Matrix[1][0] != 0 {
		t.Errorf("matrix[dog][cat] = %d, want 0", cm.Matrix[1][0])
	}
	if cm.Matrix[1][1] != 2 {
		t.Errorf("matrix[dog][dog] = %d, want 2", cm.Matrix[1][1])
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}
