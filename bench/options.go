package bench

import (
	"context"
	"encoding/binary"
	"math/rand/v2"
	"reflect"
	"time"

	"github.com/kbukum/gokit/util"
)

// RNGAlgorithm names the generator [runConfig.seededRand] drives. A named algorithm (rather than an unspecified default) is recorded in run provenance so a seed maps to the same sequence across rebuilds.
const RNGAlgorithm = "math/rand/v2:ChaCha8"

// RunMetric computes evaluation scores from predictions vs ground truth. This interface mirrors metric.Metric[L] but lives in bench to avoid an import cycle (bench/metric already imports bench). Use metric.AsRunMetric to adapt metric.Metric[L] values.
type RunMetric[L comparable] interface {
	Name() string
	Compute(scored []ScoredSample[L]) MetricResult
}

// RunContextMetric computes evaluation scores that require I/O — an embedding provider, an LLM judge — so it takes a [context.Context] for cancellation and may fail. It mirrors metric.ContextMetric[L] but lives in bench to avoid an import cycle (bench/metric already imports bench); use metric.AsRunContextMetric to adapt metric.ContextMetric[L] values. Pure, deterministic offline metrics use [RunMetric] instead.
type RunContextMetric[L comparable] interface {
	Name() string
	Compute(ctx context.Context, scored []ScoredSample[L]) (MetricResult, error)
}

// RunOption configures a BenchRunner.
type RunOption[L comparable] func(*runConfig[L])

type runConfig[L comparable] struct {
	metrics          []RunMetric[L]
	contextMetrics   []RunContextMetric[L]
	storage          RunStorage
	concurrency      int
	timeout          time.Duration
	tag              string
	clock            util.Clock
	idSuffix         func() string
	seed             uint64
	probe            ProvenanceProbe
	targets          map[string]float64
	failOnRegression bool
}

func defaultConfig[L comparable]() runConfig[L] {
	return runConfig[L]{
		concurrency: 1,
		clock:       util.SystemClock{},
		idSuffix:    randomIDSuffix,
		probe:       SystemProvenanceProbe{},
	}
}

// seededRand returns a fresh RNG seeded deterministically from the run seed. The algorithm is fixed (see [RNGAlgorithm]), so the same seed yields an identical sequence across rebuilds and distinct seeds yield distinct sequences.
func (c *runConfig[L]) seededRand() *rand.Rand {
	var seed [32]byte
	binary.LittleEndian.PutUint64(seed[:8], c.seed)
	return rand.New(rand.NewChaCha8(seed))
}

// WithMetrics configures the metrics to compute.
func WithMetrics[L comparable](metrics ...RunMetric[L]) RunOption[L] {
	return func(c *runConfig[L]) {
		c.metrics = append(c.metrics, metrics...)
	}
}

// WithContextMetrics configures I/O-backed metrics computed after the pure [RunMetric]s, each receiving the run context for cancellation. Each context metric is responsible for bounding its own remote calls; the runner imposes no per-metric timeout. Results merge deterministically: pure metrics first in registration order, then context metrics in registration order. A context metric returning an error fails the run.
func WithContextMetrics[L comparable](metrics ...RunContextMetric[L]) RunOption[L] {
	return func(c *runConfig[L]) {
		c.contextMetrics = append(c.contextMetrics, metrics...)
	}
}

// WithStorage configures the storage backend for persisting results.
func WithStorage[L comparable](s RunStorage) RunOption[L] {
	return func(c *runConfig[L]) {
		c.storage = s
	}
}

// WithConcurrency sets the number of parallel evaluation workers. Values <= 1 mean sequential execution.
func WithConcurrency[L comparable](n int) RunOption[L] {
	return func(c *runConfig[L]) {
		if n < 1 {
			n = 1
		}
		c.concurrency = n
	}
}

// WithTimeout sets the per-sample evaluation timeout.
func WithTimeout[L comparable](d time.Duration) RunOption[L] {
	return func(c *runConfig[L]) {
		c.timeout = d
	}
}

// WithTag sets a human-readable tag for the run.
func WithTag[L comparable](tag string) RunOption[L] {
	return func(c *runConfig[L]) {
		c.tag = tag
	}
}

// WithClock configures the clock used for run timestamps, IDs, and duration measurement.
func WithClock[L comparable](clock util.Clock) RunOption[L] {
	return func(c *runConfig[L]) {
		if clock != nil {
			c.clock = clock
		}
	}
}

// WithIDSuffix injects the suffix appended to untagged run IDs, making default (untagged) runs reproducible under an injected clock. The production default is a random UUID fragment; a nil source is ignored.
func WithIDSuffix[L comparable](suffix func() string) RunOption[L] {
	return func(c *runConfig[L]) {
		if suffix != nil {
			c.idSuffix = suffix
		}
	}
}

// WithTargets sets metric target thresholds (metric name → minimum value).
func WithTargets[L comparable](targets map[string]float64) RunOption[L] {
	return func(c *runConfig[L]) {
		c.targets = targets
	}
}

// WithFailOnRegression configures whether the run should fail if a regression is detected compared to the previous run.
func WithFailOnRegression[L comparable](b bool) RunOption[L] {
	return func(c *runConfig[L]) {
		c.failOnRegression = b
	}
}

// WithSeed sets the deterministic run seed recorded in provenance and driving [runConfig.seededRand]. The default seed is 0.
func WithSeed[L comparable](seed uint64) RunOption[L] {
	return func(c *runConfig[L]) {
		c.seed = seed
	}
}

// WithProvenanceProbe injects the probe used to record host and source-control identity. The default is [SystemProvenanceProbe]; tests inject a fixed probe for deterministic, offline provenance. A nil probe — including a typed-nil interface value (a nil *T boxed in the interface) — is ignored, keeping the default.
func WithProvenanceProbe[L comparable](probe ProvenanceProbe) RunOption[L] {
	return func(c *runConfig[L]) {
		if !isNilProbe(probe) {
			c.probe = probe
		}
	}
}

// isNilProbe reports whether probe is nil, including a typed-nil interface value (a nil *T stored in the interface) that a plain probe == nil check misses and that would otherwise panic when [BenchRunner.Run] dispatches a method through it.
func isNilProbe(probe ProvenanceProbe) bool {
	if probe == nil {
		return true
	}
	v := reflect.ValueOf(probe)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
