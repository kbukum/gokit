package metric

import (
	"math"

	"github.com/kbukum/gokit/bench"
)

// MAE computes Mean Absolute Error.
// Uses Sample.Label as actual and Prediction.Score as predicted.
func MAE() Metric[float64] {
	return &mae{}
}

type mae struct{}

func (m *mae) Name() string { return "mae" }

func (m *mae) Compute(scored []bench.ScoredSample[float64]) Result {
	if len(scored) == 0 {
		return Result{Name: "mae", Value: 0}
	}

	sum := 0.0
	for _, s := range scored {
		sum += math.Abs(s.Prediction.Score - s.Sample.Label)
	}

	return Result{
		Name:  "mae",
		Value: sum / float64(len(scored)),
	}
}

// MSE computes Mean Squared Error.
func MSE() Metric[float64] {
	return &mse{}
}

type mse struct{}

func (m *mse) Name() string { return "mse" }

func (m *mse) Compute(scored []bench.ScoredSample[float64]) Result {
	if len(scored) == 0 {
		return Result{Name: "mse", Value: 0}
	}

	sum := 0.0
	for _, s := range scored {
		diff := s.Prediction.Score - s.Sample.Label
		sum += diff * diff
	}

	return Result{
		Name:  "mse",
		Value: sum / float64(len(scored)),
	}
}

// RMSE computes Root Mean Squared Error.
func RMSE() Metric[float64] {
	return &rmse{}
}

type rmse struct{}

func (m *rmse) Name() string { return "rmse" }

func (m *rmse) Compute(scored []bench.ScoredSample[float64]) Result {
	if len(scored) == 0 {
		return Result{Name: "rmse", Value: 0}
	}

	sum := 0.0
	for _, s := range scored {
		diff := s.Prediction.Score - s.Sample.Label
		sum += diff * diff
	}

	return Result{
		Name:  "rmse",
		Value: math.Sqrt(sum / float64(len(scored))),
	}
}

// RSquared computes the coefficient of determination (R²).
func RSquared() Metric[float64] {
	return &rSquared{}
}

type rSquared struct{}

func (m *rSquared) Name() string { return "r_squared" }

func (m *rSquared) Compute(scored []bench.ScoredSample[float64]) Result {
	if len(scored) == 0 {
		return Result{Name: "r_squared", Value: 0}
	}

	// Compute mean of actual values.
	meanActual := 0.0
	for _, s := range scored {
		meanActual += s.Sample.Label
	}
	meanActual /= float64(len(scored))

	// Residual sum of squares and total sum of squares.
	// SSres = Σ(actual - predicted)², SStot = Σ(actual - mean)²
	ssRes := 0.0
	ssTot := 0.0
	for _, s := range scored {
		diff := s.Sample.Label - s.Prediction.Score
		ssRes += diff * diff
		diffMean := s.Sample.Label - meanActual
		ssTot += diffMean * diffMean
	}

	r2 := 1 - safeDivide(ssRes, ssTot)

	return Result{
		Name:  "r_squared",
		Value: r2,
		Values: map[string]float64{
			"ss_res": ssRes,
			"ss_tot": ssTot,
		},
	}
}
