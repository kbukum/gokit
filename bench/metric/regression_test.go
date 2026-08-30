package metric

import (
	"math"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func TestMAE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scored []bench.ScoredSample[float64]
		want   float64
	}{
		{
			name: "simple values",
			scored: []bench.ScoredSample[float64]{
				{Sample: bench.Sample[float64]{Label: 3.0}, Prediction: bench.Prediction[float64]{Score: 2.5}},
				{Sample: bench.Sample[float64]{Label: 5.0}, Prediction: bench.Prediction[float64]{Score: 5.0}},
				{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 3.0}},
			},
			want: 0.5, // mean absolute error: (0.5 + 0 + 1) / 3
		},
		{
			name: "perfect predictions",
			scored: []bench.ScoredSample[float64]{
				{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.0}},
				{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 2.0}},
			},
			want: 0.0,
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
			m := MAE()
			r := m.Compute(tt.scored)
			if r.Name != "mae" {
				t.Errorf("Name = %q, want %q", r.Name, "mae")
			}
			assertRegClose(t, "MAE", r.Value, tt.want)
		})
	}
}

func TestMSE(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 3.0}, Prediction: bench.Prediction[float64]{Score: 2.5}},
		{Sample: bench.Sample[float64]{Label: 5.0}, Prediction: bench.Prediction[float64]{Score: 5.0}},
		{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 3.0}},
	}
	// (0.25 + 0 + 1) / 3 ≈ 0.4167
	m := MSE()
	r := m.Compute(scored)
	if r.Name != "mse" {
		t.Errorf("Name = %q, want %q", r.Name, "mse")
	}
	assertRegClose(t, "MSE", r.Value, 1.25/3.0)
}

func TestMSEEmpty(t *testing.T) {
	t.Parallel()
	m := MSE()
	r := m.Compute(nil)
	assertRegClose(t, "MSE (empty)", r.Value, 0)
}

func TestRMSE(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 3.0}, Prediction: bench.Prediction[float64]{Score: 2.5}},
		{Sample: bench.Sample[float64]{Label: 5.0}, Prediction: bench.Prediction[float64]{Score: 5.0}},
		{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 3.0}},
	}

	m := RMSE()
	r := m.Compute(scored)
	if r.Name != "rmse" {
		t.Errorf("Name = %q, want %q", r.Name, "rmse")
	}
	assertRegClose(t, "RMSE", r.Value, math.Sqrt(1.25/3.0))
}

func TestRMSEEmpty(t *testing.T) {
	t.Parallel()
	m := RMSE()
	r := m.Compute(nil)
	assertRegClose(t, "RMSE (empty)", r.Value, 0)
}

func TestRSquared(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scored []bench.ScoredSample[float64]
		want   float64
	}{
		{
			name: "perfect fit",
			scored: []bench.ScoredSample[float64]{
				{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.0}},
				{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 2.0}},
				{Sample: bench.Sample[float64]{Label: 3.0}, Prediction: bench.Prediction[float64]{Score: 3.0}},
			},
			want: 1.0,
		},
		{
			name: "reasonable fit",
			scored: []bench.ScoredSample[float64]{
				{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.1}},
				{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 2.1}},
				{Sample: bench.Sample[float64]{Label: 3.0}, Prediction: bench.Prediction[float64]{Score: 2.9}},
			},
			want: 0.97, // approximate
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
			m := RSquared()
			r := m.Compute(tt.scored)
			if r.Name != "r_squared" {
				t.Errorf("Name = %q, want %q", r.Name, "r_squared")
			}
			if tt.name == "reasonable fit" {
				if r.Value < 0.95 || r.Value > 1.0 {
					t.Errorf("R² = %f, expected between 0.95 and 1.0", r.Value)
				}
			} else {
				assertRegClose(t, "R²", r.Value, tt.want)
			}
		})
	}
}

func TestRSquaredHasSSValues(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.0}},
		{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 2.0}},
	}

	m := RSquared()
	r := m.Compute(scored)

	if _, ok := r.Values["ss_res"]; !ok {
		t.Error("missing ss_res in Values")
	}
	if _, ok := r.Values["ss_tot"]; !ok {
		t.Error("missing ss_tot in Values")
	}
}

func TestRSquaredSubvalueDirections(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 1.2}},
		{Sample: bench.Sample[float64]{Label: 2.0}, Prediction: bench.Prediction[float64]{Score: 1.8}},
	}

	r := RSquared().Compute(scored)
	if r.Direction != bench.HigherIsBetter {
		t.Errorf("r_squared Direction = %v, want HigherIsBetter", r.Direction)
	}
	if got := r.Directions["ss_res"]; got != bench.LowerIsBetter {
		t.Errorf("ss_res Direction = %v, want LowerIsBetter", got)
	}
	if got := r.Directions["ss_tot"]; got != bench.Neutral {
		t.Errorf("ss_tot Direction = %v, want Neutral", got)
	}
}

func assertRegClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %.6f, want %.6f", name, got, want)
	}
}

// TestRegressionMetricDirections asserts each regression metric declares the
// optimization direction that run comparison classifies against: mae/mse/rmse
// are lower-is-better, r_squared is higher-is-better.
func TestRegressionMetricDirections(t *testing.T) {
	t.Parallel()

	scored := []bench.ScoredSample[float64]{
		{Sample: bench.Sample[float64]{Label: 1.0}, Prediction: bench.Prediction[float64]{Score: 0.8}},
		{Sample: bench.Sample[float64]{Label: 0.0}, Prediction: bench.Prediction[float64]{Score: 0.2}},
	}

	tests := []struct {
		name   string
		metric Metric[float64]
		want   bench.Direction
	}{
		{"mae", MAE(), bench.LowerIsBetter},
		{"mse", MSE(), bench.LowerIsBetter},
		{"rmse", RMSE(), bench.LowerIsBetter},
		{"r_squared", RSquared(), bench.HigherIsBetter},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.metric.Compute(scored).Direction; got != tc.want {
				t.Errorf("%s Direction = %v, want %v", tc.name, got, tc.want)
			}
			// An empty input must carry the same direction as a populated one.
			if got := tc.metric.Compute(nil).Direction; got != tc.want {
				t.Errorf("%s empty Direction = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
