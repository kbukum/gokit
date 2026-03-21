package metric

import (
	"fmt"

	"github.com/kbukum/gokit/bench"
)

// ClassificationOption configures classification metrics.
type ClassificationOption func(*classificationConfig)

type classificationConfig struct {
	threshold float64
}

// WithThreshold sets the decision threshold (default: 0.5).
func WithThreshold(t float64) ClassificationOption {
	return func(c *classificationConfig) { c.threshold = t }
}

// BinaryClassification computes precision, recall, F1, accuracy, and FPR
// for binary classification. positiveLabel is the label considered "positive".
func BinaryClassification[L comparable](positiveLabel L, opts ...ClassificationOption) Metric[L] {
	cfg := classificationConfig{threshold: 0.5}
	for _, o := range opts {
		o(&cfg)
	}
	return &binaryClassification[L]{positive: positiveLabel, threshold: cfg.threshold}
}

type binaryClassification[L comparable] struct {
	positive  L
	threshold float64
}

func (m *binaryClassification[L]) Name() string { return "classification" }

func (m *binaryClassification[L]) Compute(scored []bench.ScoredSample[L]) Result {
	var tp, fp, tn, fn int
	for _, s := range scored {
		actual := s.Sample.Label == m.positive
		predicted := s.Prediction.Score >= m.threshold

		switch {
		case actual && predicted:
			tp++
		case !actual && predicted:
			fp++
		case actual && !predicted:
			fn++
		default:
			tn++
		}
	}

	precision := safeDivide(float64(tp), float64(tp+fp))
	recall := safeDivide(float64(tp), float64(tp+fn))
	f1 := safeDivide(2*precision*recall, precision+recall)
	accuracy := safeDivide(float64(tp+tn), float64(len(scored)))
	fpr := safeDivide(float64(fp), float64(fp+tn))

	// Build confusion matrix labels
	labels := make([]string, 0, 2)
	labels = append(labels, fmt.Sprintf("%v", m.positive))
	var negLabel L
	for _, s := range scored {
		if s.Sample.Label != m.positive {
			negLabel = s.Sample.Label
			break
		}
	}
	labels = append(labels, fmt.Sprintf("%v", negLabel))
	matrix := [][]int{{tp, fn}, {fp, tn}}

	values := map[string]float64{
		"precision": precision,
		"recall":    recall,
		"f1":        f1,
		"accuracy":  accuracy,
		"fpr":       fpr,
		"tp":        float64(tp),
		"fp":        float64(fp),
		"tn":        float64(tn),
		"fn":        float64(fn),
		"threshold": m.threshold,
	}

	return Result{
		Name:   "classification",
		Value:  f1,
		Values: values,
		Detail: bench.ConfusionMatrixDetail{
			Labels:      labels,
			Matrix:      matrix,
			Orientation: "row=actual, col=predicted",
		},
	}
}

// ConfusionMatrix computes the full confusion matrix for any number of labels.
func ConfusionMatrix[L comparable](labels []L) Metric[L] {
	return &confusionMatrix[L]{labels: labels}
}

type confusionMatrix[L comparable] struct {
	labels []L
}

func (m *confusionMatrix[L]) Name() string { return "confusion_matrix" }

func (m *confusionMatrix[L]) Compute(scored []bench.ScoredSample[L]) Result {
	labelIndex := make(map[L]int)
	for i, l := range m.labels {
		labelIndex[l] = i
	}
	n := len(m.labels)
	matrix := make([][]int, n)
	for i := range matrix {
		matrix[i] = make([]int, n)
	}

	for _, s := range scored {
		actual, aOk := labelIndex[s.Sample.Label]
		predicted, pOk := labelIndex[s.Prediction.Label]
		if aOk && pOk {
			matrix[actual][predicted]++
		}
	}

	labelStrings := make([]string, n)
	for i, l := range m.labels {
		labelStrings[i] = fmt.Sprintf("%v", l)
	}

	return Result{
		Name:  "confusion_matrix",
		Value: 0,
		Detail: bench.ConfusionMatrixDetail{
			Labels:      labelStrings,
			Matrix:      matrix,
			Orientation: "row=actual, col=predicted",
		},
	}
}

// ThresholdSweep computes classification metrics at multiple thresholds.
func ThresholdSweep[L comparable](positiveLabel L, thresholds []float64) Metric[L] {
	if thresholds == nil {
		thresholds = []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}
	}
	return &thresholdSweep[L]{positive: positiveLabel, thresholds: thresholds}
}

type thresholdSweep[L comparable] struct {
	positive   L
	thresholds []float64
}

func (m *thresholdSweep[L]) Name() string { return "threshold_sweep" }

func (m *thresholdSweep[L]) Compute(scored []bench.ScoredSample[L]) Result {
	points := make([]bench.ThresholdPoint, 0, len(m.thresholds))
	var bestF1 float64
	for _, t := range m.thresholds {
		var tp, fp, tn, fn int
		for _, s := range scored {
			actual := s.Sample.Label == m.positive
			predicted := s.Prediction.Score >= t
			switch {
			case actual && predicted:
				tp++
			case !actual && predicted:
				fp++
			case actual && !predicted:
				fn++
			default:
				tn++
			}
		}
		precision := safeDivide(float64(tp), float64(tp+fp))
		recall := safeDivide(float64(tp), float64(tp+fn))
		f1 := safeDivide(2*precision*recall, precision+recall)
		accuracy := safeDivide(float64(tp+tn), float64(len(scored)))

		if f1 > bestF1 {
			bestF1 = f1
		}
		points = append(points, bench.ThresholdPoint{
			Threshold: t,
			Precision: precision,
			Recall:    recall,
			F1:        f1,
			Accuracy:  accuracy,
		})
	}

	return Result{
		Name:   "threshold_sweep",
		Value:  bestF1,
		Detail: points,
	}
}

// MultiClassClassification computes macro/micro/weighted averages for multi-class.
func MultiClassClassification[L comparable](labels []L) Metric[L] {
	return &multiClassClassification[L]{labels: labels}
}

type multiClassClassification[L comparable] struct {
	labels []L
}

func (m *multiClassClassification[L]) Name() string { return "multi_class_classification" }

func (m *multiClassClassification[L]) Compute(scored []bench.ScoredSample[L]) Result {
	n := len(m.labels)
	labelIndex := make(map[L]int, n)
	for i, l := range m.labels {
		labelIndex[l] = i
	}

	// Per-class TP/FP/FN
	tp := make([]int, n)
	fp := make([]int, n)
	fn := make([]int, n)

	for _, s := range scored {
		actual, aOk := labelIndex[s.Sample.Label]
		predicted, pOk := labelIndex[s.Prediction.Label]
		if !aOk || !pOk {
			continue
		}
		if actual == predicted {
			tp[actual]++
		} else {
			fn[actual]++
			fp[predicted]++
		}
	}

	// Macro average
	var macroPrecision, macroRecall, macroF1 float64
	classCount := 0
	for i := 0; i < n; i++ {
		p := safeDivide(float64(tp[i]), float64(tp[i]+fp[i]))
		r := safeDivide(float64(tp[i]), float64(tp[i]+fn[i]))
		f := safeDivide(2*p*r, p+r)
		if tp[i]+fp[i]+fn[i] > 0 {
			macroPrecision += p
			macroRecall += r
			macroF1 += f
			classCount++
		}
	}
	if classCount > 0 {
		macroPrecision /= float64(classCount)
		macroRecall /= float64(classCount)
		macroF1 /= float64(classCount)
	}

	// Micro average
	totalTP, totalFP, totalFN := 0, 0, 0
	for i := 0; i < n; i++ {
		totalTP += tp[i]
		totalFP += fp[i]
		totalFN += fn[i]
	}
	microPrecision := safeDivide(float64(totalTP), float64(totalTP+totalFP))
	microRecall := safeDivide(float64(totalTP), float64(totalTP+totalFN))
	microF1 := safeDivide(2*microPrecision*microRecall, microPrecision+microRecall)

	// Overall accuracy
	correct := 0
	for _, s := range scored {
		if s.Sample.Label == s.Prediction.Label {
			correct++
		}
	}
	accuracy := safeDivide(float64(correct), float64(len(scored)))

	values := map[string]float64{
		"macro_precision": macroPrecision,
		"macro_recall":    macroRecall,
		"macro_f1":        macroF1,
		"micro_precision": microPrecision,
		"micro_recall":    microRecall,
		"micro_f1":        microF1,
		"accuracy":        accuracy,
	}

	return Result{
		Name:   "multi_class_classification",
		Value:  macroF1,
		Values: values,
	}
}

func safeDivide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
