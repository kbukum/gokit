package metric

import (
	"math"
	"sort"

	"github.com/kbukum/gokit/bench"
)

// AUCROC computes Area Under ROC Curve for binary classification.
// positiveLabel is the positive class. Stores bench.ROCCurve in Detail.
func AUCROC[L comparable](positiveLabel L) Metric[L] {
	return &aucroc[L]{positive: positiveLabel}
}

type aucroc[L comparable] struct {
	positive L
}

func (m *aucroc[L]) Name() string { return "aucroc" }

func (m *aucroc[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "aucroc", Value: 0}
	}

	// Sort by score descending.
	sorted := make([]bench.ScoredSample[L], len(scored))
	copy(sorted, scored)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Prediction.Score > sorted[j].Prediction.Score
	})

	totalPos, totalNeg := 0, 0
	for _, s := range sorted {
		if s.Sample.Label == m.positive {
			totalPos++
		} else {
			totalNeg++
		}
	}

	if totalPos == 0 || totalNeg == 0 {
		return Result{Name: "aucroc", Value: 0}
	}

	// Walk through sorted samples, accumulating TPR/FPR at each threshold.
	fprSlice := []float64{0}
	tprSlice := []float64{0}
	thresholds := []float64{math.Inf(1)}

	tp, fp := 0, 0
	for _, s := range sorted {
		if s.Sample.Label == m.positive {
			tp++
		} else {
			fp++
		}
		fprSlice = append(fprSlice, float64(fp)/float64(totalNeg))
		tprSlice = append(tprSlice, float64(tp)/float64(totalPos))
		thresholds = append(thresholds, s.Prediction.Score)
	}

	// Trapezoidal rule for AUC.
	auc := 0.0
	for i := 1; i < len(fprSlice); i++ {
		dx := fprSlice[i] - fprSlice[i-1]
		auc += dx * (tprSlice[i] + tprSlice[i-1]) / 2
	}

	return Result{
		Name:  "aucroc",
		Value: auc,
		Detail: bench.ROCCurve{
			FPR:        fprSlice,
			TPR:        tprSlice,
			Thresholds: thresholds,
			AUC:        auc,
		},
	}
}

// BrierScore computes the Brier score (mean squared difference between
// predicted probability and actual outcome). Lower is better. Range: [0, 1].
func BrierScore[L comparable](positiveLabel L) Metric[L] {
	return &brierScore[L]{positive: positiveLabel}
}

type brierScore[L comparable] struct {
	positive L
}

func (m *brierScore[L]) Name() string { return "brier_score" }

func (m *brierScore[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "brier_score", Value: 0}
	}

	sum := 0.0
	for _, s := range scored {
		actual := 0.0
		if s.Sample.Label == m.positive {
			actual = 1.0
		}
		diff := s.Prediction.Score - actual
		sum += diff * diff
	}

	return Result{
		Name:  "brier_score",
		Value: sum / float64(len(scored)),
	}
}

// LogLoss computes the logarithmic loss (cross-entropy loss).
func LogLoss[L comparable](positiveLabel L) Metric[L] {
	return &logLoss[L]{positive: positiveLabel}
}

type logLoss[L comparable] struct {
	positive L
}

func (m *logLoss[L]) Name() string { return "log_loss" }

func (m *logLoss[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "log_loss", Value: 0}
	}

	const epsilon = 1e-15
	sum := 0.0
	for _, s := range scored {
		actual := 0.0
		if s.Sample.Label == m.positive {
			actual = 1.0
		}
		p := math.Max(epsilon, math.Min(1-epsilon, s.Prediction.Score))
		sum += actual*math.Log(p) + (1-actual)*math.Log(1-p)
	}

	return Result{
		Name:  "log_loss",
		Value: -sum / float64(len(scored)),
	}
}

// Calibration computes a calibration curve (predicted probability vs actual frequency).
// bins is the number of bins (default 10).
func Calibration[L comparable](positiveLabel L, bins int) Metric[L] {
	if bins <= 0 {
		bins = 10
	}
	return &calibration[L]{positive: positiveLabel, bins: bins}
}

type calibration[L comparable] struct {
	positive L
	bins     int
}

func (m *calibration[L]) Name() string { return "calibration" }

func (m *calibration[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "calibration", Value: 0}
	}

	binWidth := 1.0 / float64(m.bins)
	predictedProb := make([]float64, m.bins)
	actualFreq := make([]float64, m.bins)
	binCount := make([]int, m.bins)
	binPosCount := make([]int, m.bins)
	binScoreSum := make([]float64, m.bins)

	for _, s := range scored {
		idx := int(s.Prediction.Score / binWidth)
		if idx >= m.bins {
			idx = m.bins - 1
		}
		if idx < 0 {
			idx = 0
		}
		binCount[idx]++
		binScoreSum[idx] += s.Prediction.Score
		if s.Sample.Label == m.positive {
			binPosCount[idx]++
		}
	}

	// Expected Calibration Error (ECE).
	ece := 0.0
	total := float64(len(scored))
	for i := 0; i < m.bins; i++ {
		if binCount[i] > 0 {
			predictedProb[i] = binScoreSum[i] / float64(binCount[i])
			actualFreq[i] = float64(binPosCount[i]) / float64(binCount[i])
			ece += (float64(binCount[i]) / total) * math.Abs(actualFreq[i]-predictedProb[i])
		}
	}

	return Result{
		Name:  "calibration",
		Value: ece,
		Detail: bench.CalibrationCurve{
			PredictedProbability: predictedProb,
			ActualFrequency:      actualFreq,
			BinCount:             binCount,
		},
	}
}
