package bench

// ROCCurve holds receiver operating characteristic curve data.
type ROCCurve struct {
	FPR        []float64
	TPR        []float64
	Thresholds []float64
	AUC        float64
}

// PrecisionRecallCurve holds precision-recall curve data.
type PrecisionRecallCurve struct {
	Precision  []float64
	Recall     []float64
	Thresholds []float64
}

// CalibrationCurve holds calibration curve data.
type CalibrationCurve struct {
	PredictedProbability []float64
	ActualFrequency      []float64
	BinCount             []int
}

// ConfusionMatrixDetail holds confusion matrix data.
type ConfusionMatrixDetail struct {
	Labels      []string
	Matrix      [][]int
	Orientation string // "row=actual, col=predicted"
}

// ScoreDistribution holds a histogram of scores for a label.
type ScoreDistribution struct {
	Label  string
	Bins   []float64
	Counts []int
}

// ThresholdPoint holds classification metrics at a specific threshold.
type ThresholdPoint struct {
	Threshold float64
	Precision float64
	Recall    float64
	F1        float64
	Accuracy  float64
}
