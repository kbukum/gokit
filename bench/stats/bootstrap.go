package stats

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	apperrors "github.com/kbukum/gokit/errors"
)

// Limits on inputs.
const (
	MaxValues    = 100_000
	MaxResamples = 10_000
)

// Interval is an estimate with a two-sided confidence interval over N values. A single value has Low == High == Estimate.
type Interval struct {
	Estimate float64
	Low      float64
	High     float64
	N        int
}

// Width is High − Low.
func (i Interval) Width() float64 { return i.High - i.Low }

// Bootstrap configures a percentile bootstrap: the number of resamples, the confidence level in (0, 1) and the seed.
type Bootstrap struct {
	Resamples  int
	Confidence float64
	Seed       uint64
}

// DefaultBootstrap is 2000 resamples at 95% confidence with the given seed.
func DefaultBootstrap(seed uint64) Bootstrap {
	return Bootstrap{Resamples: 2000, Confidence: 0.95, Seed: seed}
}

func (b Bootstrap) validate() error {
	if b.Resamples < 1 || b.Resamples > MaxResamples {
		return apperrors.InvalidInput("bootstrap", fmt.Sprintf("resamples must be 1 to %d", MaxResamples))
	}
	if !(b.Confidence > 0 && b.Confidence < 1) {
		return apperrors.InvalidInput("bootstrap", "confidence must be between 0 and 1")
	}
	return nil
}

func (b Bootstrap) rand() *rand.Rand {
	var seed [32]byte
	binary.LittleEndian.PutUint64(seed[:], b.Seed)
	return rand.New(rand.NewChaCha8(seed))
}

// BootstrapMean returns the mean of values with a percentile bootstrap interval. It needs 1 to [MaxValues] finite values.
func BootstrapMean(values []float64, b Bootstrap) (Interval, error) {
	if err := b.validate(); err != nil {
		return Interval{}, err
	}
	if len(values) == 0 || len(values) > MaxValues {
		return Interval{}, apperrors.InvalidInput("values", fmt.Sprintf("need 1 to %d values", MaxValues))
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Interval{}, apperrors.InvalidInput("values", "values must be finite")
		}
	}
	estimate := mean(values)
	if len(values) == 1 {
		return Interval{Estimate: estimate, Low: estimate, High: estimate, N: 1}, nil
	}
	r := b.rand()
	means := make([]float64, b.Resamples)
	n := len(values)
	for i := range means {
		var sum float64
		for range n {
			sum += values[r.IntN(n)]
		}
		means[i] = sum / float64(n)
	}
	low, high := percentiles(means, b.Confidence)
	return Interval{Estimate: estimate, Low: low, High: high, N: n}, nil
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// percentiles sorts xs and returns its (1−c)/2 and (1+c)/2 quantiles by nearest rank.
func percentiles(xs []float64, confidence float64) (low, high float64) {
	slices.Sort(xs)
	alpha := (1 - confidence) / 2
	at := func(q float64) float64 {
		i := int(math.Ceil(q*float64(len(xs)))) - 1
		return xs[min(max(i, 0), len(xs)-1)]
	}
	return at(alpha), at(1 - alpha)
}
