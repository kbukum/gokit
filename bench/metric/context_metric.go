package metric

import (
	"context"

	"github.com/kbukum/gokit/bench"
)

// ContextMetric computes an evaluation score that requires I/O — embedding a prediction, calling an LLM judge — so it takes a [context.Context] for cancellation and may fail. It is the Go-idiomatic mirror of an async metric: there is no async/await, an I/O-backed metric simply threads a context and returns an error.
//
// Implementations are responsible for bounding their own remote calls (for example through a [resilience.Policy], as [SemanticSimilarity] does): the runner forwards the run context for cancellation but imposes no per-metric timeout, so a metric that does not bound its calls can stall a run.
//
// Pure, deterministic offline metrics implement [Metric] instead. A resolved context-metric result can also be surfaced as a pure [Metric] with [AsSync] (the precompute path), and adapted to the runner's [bench.RunContextMetric] with [AsRunContextMetric].
type ContextMetric[L comparable] interface {
	Name() string
	Compute(ctx context.Context, scored []bench.ScoredSample[L]) (Result, error)
}

// runContextMetricAdapter adapts a [ContextMetric] to a [bench.RunContextMetric].
type runContextMetricAdapter[L comparable] struct {
	m ContextMetric[L]
}

// AsRunContextMetric converts a [ContextMetric] into a [bench.RunContextMetric] for use with [bench.BenchRunner] via [bench.WithContextMetrics].
func AsRunContextMetric[L comparable](m ContextMetric[L]) bench.RunContextMetric[L] {
	return &runContextMetricAdapter[L]{m: m}
}

// AsRunContextMetrics converts multiple [ContextMetric] values into [bench.RunContextMetric].
func AsRunContextMetrics[L comparable](metrics ...ContextMetric[L]) []bench.RunContextMetric[L] {
	out := make([]bench.RunContextMetric[L], len(metrics))
	for i, m := range metrics {
		out[i] = AsRunContextMetric[L](m)
	}
	return out
}

func (a *runContextMetricAdapter[L]) Name() string { return a.m.Name() }

func (a *runContextMetricAdapter[L]) Compute(ctx context.Context, scored []bench.ScoredSample[L]) (bench.MetricResult, error) {
	r, err := a.m.Compute(ctx, scored)
	if err != nil {
		return bench.MetricResult{}, err
	}
	return bench.MetricResult{
		Name:      r.Name,
		Value:     r.Value,
		Values:    r.Values,
		Detail:    r.Detail,
		Direction: r.Direction,
	}, nil
}

// AsSync surfaces an already-resolved [Result] as a pure [Metric] (the precompute path): its Compute ignores its input and returns the precomputed result. Use it when a context-metric result has been computed out of band and should join a [Suite] of deterministic metrics; the context-metric and precompute paths yield an identical [Result].
func AsSync[L comparable](precomputed Result) Metric[L] {
	return &precomputedMetric[L]{result: precomputed}
}

type precomputedMetric[L comparable] struct {
	result Result
}

func (m *precomputedMetric[L]) Name() string { return m.result.Name }

func (m *precomputedMetric[L]) Compute([]bench.ScoredSample[L]) Result { return m.result }
