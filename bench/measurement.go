package bench

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"
)

// Accumulator computes a metric with bounded state, one observation at a time.
type Accumulator[L comparable] interface {
	Add(ScoredSample[L]) error
	Result() MetricResult
}

// RunStreaming creates independent accumulators for each branch and run.
type RunStreaming[L comparable] interface {
	RunMetric[L]
	NewAccumulator() Accumulator[L]
}

func (r *BenchRunner[L]) measure(ctx context.Context, w *runWriter, branch int) (metrics []MetricResult, performance PerformanceSummary, tokenizers []string, resultErr error) {
	accumulators := make([]Accumulator[L], len(r.cfg.metrics))
	retain := len(r.cfg.contextMetrics) != 0
	for i, m := range r.cfg.metrics {
		if streaming, ok := m.(RunStreaming[L]); ok {
			accumulators[i] = streaming.NewAccumulator()
		} else {
			retain = true
		}
	}
	var retained []ScoredSample[L]
	var latency, ttft []uint64
	var tally performanceTally
	counters := make(map[string]bool)
	it := w.iterator(branch)
	defer func() { resultErr = errors.Join(resultErr, it.Close()) }()
	for {
		record, ok, err := it.Next(ctx)
		if err != nil {
			return nil, performance, nil, err
		}
		if !ok {
			break
		}
		scored, err := decodeObservation[L](record)
		if err != nil {
			return nil, performance, nil, err
		}
		for _, accumulator := range accumulators {
			if accumulator != nil {
				if err := accumulator.Add(scored); err != nil {
					return nil, performance, nil, &RunError{Outcome: OutcomeMetricFailed, Cause: err}
				}
			}
		}
		if retain {
			if len(retained) >= r.cfg.limits.MaxRetainedSamples {
				return nil, performance, nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("whole-set metrics exceed retained-observation limit %d", r.cfg.limits.MaxRetainedSamples)}
			}
			retained = append(retained, scored)
		}
		if record.Usage.Source == UsageCounted {
			counters[record.Usage.Tokenizer] = true
		}
		if len(counters) > r.cfg.limits.MaxLabels {
			return nil, performance, nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("too many tokenizer identities")}
		}
		if err := tally.add(Measurement{Latency: record.Latency, TTFT: record.TTFT, Usage: record.Usage, Failed: record.Error != ""}); err != nil {
			return nil, performance, nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: err}
		}
		if record.Error != "" {
			continue
		}
		if tally.summary.Latency.Count <= r.cfg.percentileThreshold {
			latency = append(latency, uint64(record.Latency))
		} else {
			latency = nil
		}
		if record.TTFT != nil {
			if tally.ttftCount <= r.cfg.percentileThreshold {
				ttft = append(ttft, uint64(*record.TTFT))
			} else {
				ttft = nil
			}
		}
	}
	performance = tally.summary
	for i, m := range r.cfg.metrics {
		if err := ctx.Err(); err != nil {
			return nil, performance, nil, err
		}
		if accumulators[i] != nil {
			metrics = append(metrics, accumulators[i].Result())
		} else {
			metrics = append(metrics, m.Compute(retained))
		}
	}
	for _, m := range r.cfg.contextMetrics {
		result, err := m.Compute(ctx, retained)
		if err != nil {
			return nil, performance, nil, &RunError{Outcome: OutcomeMetricFailed, Cause: err}
		}
		metrics = append(metrics, result)
	}
	quantiles, err := exactQuantiles(ctx, w, branch, tally.summary.Latency.Count, tally.ttftCount, latency, ttft)
	if err != nil {
		return nil, performance, nil, err
	}
	performance = tally.finish(quantiles)
	metrics = append(metrics, performance.Metrics()...)
	for counter := range counters {
		tokenizers = append(tokenizers, counter)
	}
	slices.Sort(tokenizers)
	return metrics, performance, tokenizers, nil
}

// Measurement is one externally observed invocation. Failed measurements count toward failure_rate only, as failed
// observations do in a run.
type Measurement struct {
	Latency time.Duration
	TTFT    *time.Duration
	Usage   TokenUsage
	Failed  bool
}

// MaxMeasurements bounds the measurements [SummarizePerformance] sorts in memory.
const MaxMeasurements = 4096

// SummarizePerformance summarizes externally collected measurements exactly as a run summarizes its observations:
// nearest-rank percentiles over successful measurements, TTFT only where observed, and token totals only from
// measurements with usage. Pass the result's [PerformanceSummary.Metrics] to bounds or comparisons. It refuses
// negative durations or tokens, an unknown usage source, more than [MaxMeasurements] measurements and token totals
// that overflow.
func SummarizePerformance(measurements []Measurement) (PerformanceSummary, error) {
	if len(measurements) > MaxMeasurements {
		return PerformanceSummary{}, fmt.Errorf("bench: %d measurements exceed the limit of %d", len(measurements), MaxMeasurements)
	}
	var tally performanceTally
	latency := make([]uint64, 0, len(measurements))
	var ttft []uint64
	for i, m := range measurements {
		if m.Latency < 0 || (m.TTFT != nil && *m.TTFT < 0) || m.Usage.Input < 0 || m.Usage.Output < 0 {
			return PerformanceSummary{}, fmt.Errorf("bench: measurement %d has a negative duration or token count", i)
		}
		switch m.Usage.Source {
		case UsageUnavailable, UsageReported, UsageCounted:
		default:
			return PerformanceSummary{}, fmt.Errorf("bench: measurement %d has unknown usage source %q", i, m.Usage.Source)
		}
		if err := tally.add(m); err != nil {
			return PerformanceSummary{}, err
		}
		if m.Failed {
			continue
		}
		latency = append(latency, uint64(m.Latency))
		if m.TTFT != nil {
			ttft = append(ttft, uint64(*m.TTFT))
		}
	}
	return tally.finish([2][3]uint64{nearestRanks(latency), nearestRanks(ttft)}), nil
}

// performanceTally accumulates a [PerformanceSummary] one measurement at a time; finish adds the percentiles.
type performanceTally struct {
	summary                           PerformanceSummary
	latencySum, ttftSum, usageSeconds float64
	ttftCount                         int
}

func (t *performanceTally) add(m Measurement) error {
	t.summary.Observations++
	if m.Failed {
		t.summary.Failures++
		return nil
	}
	t.summary.Latency.Count++
	t.latencySum += float64(m.Latency)
	if m.TTFT != nil {
		t.ttftCount++
		t.ttftSum += float64(*m.TTFT)
	}
	if m.Usage.Source == UsageUnavailable {
		t.summary.WithoutUsage++
		return nil
	}
	if m.Usage.Input > math.MaxInt64-t.summary.InputTokens || m.Usage.Output > math.MaxInt64-t.summary.OutputTokens {
		return fmt.Errorf("token usage totals overflow")
	}
	t.summary.InputTokens += m.Usage.Input
	t.summary.OutputTokens += m.Usage.Output
	t.usageSeconds += m.Latency.Seconds()
	return nil
}

func (t *performanceTally) finish(quantiles [2][3]uint64) PerformanceSummary {
	p := t.summary
	p.Latency = percentileSummary(p.Latency.Count, t.latencySum, quantiles[0])
	p.TTFT = nil
	if t.ttftCount != 0 {
		ttft := percentileSummary(t.ttftCount, t.ttftSum, quantiles[1])
		p.TTFT = &ttft
	}
	if t.usageSeconds > 0 {
		p.OutputTPS = float64(p.OutputTokens) / t.usageSeconds
		p.TotalTPS = (float64(p.InputTokens) + float64(p.OutputTokens)) / t.usageSeconds
	}
	return p
}

// nearestRanks sorts values and returns their P50, P95 and P99 by nearest rank, ceil(p*n/100).
func nearestRanks(values []uint64) [3]uint64 {
	var out [3]uint64
	if len(values) == 0 {
		return out
	}
	slices.Sort(values)
	for q, percentile := range []int{50, 95, 99} {
		out[q] = values[nearestRank(percentile, len(values))-1]
	}
	return out
}

func nearestRank(percentile, count int) int {
	return int(math.Ceil(float64(percentile) * float64(count) / 100))
}

func percentileSummary(count int, sum float64, values [3]uint64) Percentiles {
	p := Percentiles{Count: count, P50: float64(values[0]) / 1e6, P95: float64(values[1]) / 1e6, P99: float64(values[2]) / 1e6}
	if count != 0 {
		p.Mean = sum / float64(count) / 1e6
	}
	return p
}

// Metrics returns the summary as metric results: latency_ms, ttft_ms when observed, throughput_output_tps,
// throughput_total_tps, failure_rate and usage_tokens.
func (p PerformanceSummary) Metrics() []MetricResult {
	latency := func(name string, v Percentiles) MetricResult {
		return MetricResult{Name: name, Value: v.P50, Direction: LowerIsBetter, Values: map[string]float64{"p50": v.P50, "p95": v.P95, "p99": v.P99, "mean": v.Mean}}
	}
	out := []MetricResult{latency("latency_ms", p.Latency)}
	if p.TTFT != nil {
		out = append(out, latency("ttft_ms", *p.TTFT))
	}
	failureRate := 0.0
	if p.Observations > 0 {
		failureRate = float64(p.Failures) / float64(p.Observations)
	}
	return append(out,
		MetricResult{Name: "throughput_output_tps", Value: p.OutputTPS},
		MetricResult{Name: "throughput_total_tps", Value: p.TotalTPS},
		MetricResult{Name: "failure_rate", Value: failureRate, Direction: LowerIsBetter},
		MetricResult{Name: "usage_tokens", Direction: Neutral, Value: float64(p.InputTokens) + float64(p.OutputTokens), Values: map[string]float64{"input": float64(p.InputTokens), "output": float64(p.OutputTokens), "without_usage": float64(p.WithoutUsage)}},
	)
}

type radixTarget struct {
	kind     int
	quantile int
	rank     int
	prefix   uint64
	buckets  [256]int
}

// exactQuantiles solves every latency/TTFT rank together in eight storage passes.
func exactQuantiles(ctx context.Context, w *runWriter, branch, latencyCount, ttftCount int, latency, ttft []uint64) ([2][3]uint64, error) {
	var out [2][3]uint64
	var targets []radixTarget
	for kind, count := range []int{latencyCount, ttftCount} {
		if count == 0 {
			continue
		}
		values := latency
		if kind == 1 {
			values = ttft
		}
		if len(values) == count {
			out[kind] = nearestRanks(values)
			continue
		}
		for q, percentile := range []int{50, 95, 99} {
			targets = append(targets, radixTarget{kind: kind, quantile: q, rank: nearestRank(percentile, count)})
		}
	}
	if len(targets) == 0 {
		return out, nil
	}
	for pass := 0; pass < 8; pass++ {
		shift := uint(56 - pass*8)
		var mask uint64
		if pass == 0 {
			mask = 0
		} else {
			mask = ^uint64(0) << (shift + 8)
		}
		it := w.iterator(branch)
		for {
			record, ok, err := it.Next(ctx)
			if err != nil {
				return out, errors.Join(err, it.Close())
			}
			if !ok {
				break
			}
			if record.Error != "" {
				continue
			}
			for i := range targets {
				target := &targets[i]
				value := uint64(record.Latency)
				if target.kind == 1 {
					if record.TTFT == nil {
						continue
					}
					value = uint64(*record.TTFT)
				}
				if value&mask == target.prefix {
					target.buckets[(value>>shift)&255]++
				}
			}
		}
		if err := it.Close(); err != nil {
			return out, err
		}
		for i := range targets {
			target := &targets[i]
			for bucket, count := range &target.buckets {
				if target.rank > count {
					target.rank -= count
					continue
				}
				target.prefix |= uint64(bucket) << shift
				break
			}
			target.buckets = [256]int{}
		}
	}
	for i := range targets {
		target := &targets[i]
		out[target.kind][target.quantile] = target.prefix
	}
	return out, nil
}
