package metric

import (
	"math"

	"github.com/kbukum/gokit/bench"
)

type meanAccumulator[L comparable] struct {
	name      string
	count     int
	sum       float64
	extra     float64
	direction bench.Direction
	sample    func(bench.ScoredSample[L]) (float64, float64)
	sqrt      bool
}

func (a *meanAccumulator[L]) Add(sample bench.ScoredSample[L]) error {
	value, extra := a.sample(sample)
	a.count++
	a.sum += value
	a.extra += extra
	return nil
}

func (a *meanAccumulator[L]) Result() Result {
	r := Result{Name: a.name, Direction: a.direction}
	if a.count != 0 {
		r.Value = a.sum / float64(a.count)
		if a.sqrt {
			r.Value = math.Sqrt(r.Value)
		}
		if a.name != "exact_match" && a.direction == bench.HigherIsBetter {
			r.Values = map[string]float64{"mean_similarity": a.extra / float64(a.count)}
		}
	}
	return r
}

func (m *exactMatch[L]) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: m.Name(), Version: "1"}
}

func (m *exactMatch[L]) NewAccumulator() bench.Accumulator[L] {
	return &meanAccumulator[L]{name: m.Name(), sample: func(s bench.ScoredSample[L]) (float64, float64) {
		if s.Error == "" && s.Sample.Label == s.Prediction.Label {
			return 1, 0
		}
		return 0, 0
	}}
}

func (m *fuzzyMatch) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: m.Name(), Version: "1", Config: map[string]string{"threshold": formatThreshold(m.threshold)}}
}

func (m *fuzzyMatch) NewAccumulator() bench.Accumulator[string] {
	return &meanAccumulator[string]{name: m.Name(), sample: func(s bench.ScoredSample[string]) (float64, float64) {
		sim := levenshteinSimilarity(s.Sample.Label, s.Prediction.Label)
		if sim >= m.threshold {
			return 1, sim
		}
		return 0, sim
	}}
}

func (m *mae) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: m.Name(), Version: "1"}
}

func (m *mse) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: m.Name(), Version: "1"}
}

func (m *rmse) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: m.Name(), Version: "1"}
}

func regressionAccumulator(name string, absolute, root bool) bench.Accumulator[float64] {
	return &meanAccumulator[float64]{name: name, direction: bench.LowerIsBetter, sqrt: root, sample: func(s bench.ScoredSample[float64]) (float64, float64) {
		d := s.Prediction.Score - s.Sample.Label
		if absolute {
			return math.Abs(d), 0
		}
		return d * d, 0
	}}
}

func (m *mae) NewAccumulator() bench.Accumulator[float64] {
	return regressionAccumulator(m.Name(), true, false)
}

func (m *mse) NewAccumulator() bench.Accumulator[float64] {
	return regressionAccumulator(m.Name(), false, false)
}

func (m *rmse) NewAccumulator() bench.Accumulator[float64] {
	return regressionAccumulator(m.Name(), false, true)
}
