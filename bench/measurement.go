package bench

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
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
	var latencySum, ttftSum, usageSeconds float64
	var ttftCount int
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
		performance.Observations++
		if record.Usage.Source == UsageCounted {
			counters[record.Usage.Tokenizer] = true
		}
		if len(counters) > r.cfg.limits.MaxLabels {
			return nil, performance, nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("too many tokenizer identities")}
		}
		if record.Error != "" {
			performance.Failures++
			continue
		}
		performance.Latency.Count++
		latencySum += float64(record.Latency)
		if performance.Latency.Count <= r.cfg.percentileThreshold {
			latency = append(latency, uint64(record.Latency))
		} else {
			latency = nil
		}
		if record.TTFT != nil {
			ttftCount++
			ttftSum += float64(*record.TTFT)
			if ttftCount <= r.cfg.percentileThreshold {
				ttft = append(ttft, uint64(*record.TTFT))
			} else {
				ttft = nil
			}
		}
		if record.Usage.Source == UsageUnavailable {
			performance.WithoutUsage++
			continue
		}
		if record.Usage.Input > math.MaxInt64-performance.InputTokens || record.Usage.Output > math.MaxInt64-performance.OutputTokens {
			return nil, performance, nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: fmt.Errorf("token usage totals overflow")}
		}
		performance.InputTokens += record.Usage.Input
		performance.OutputTokens += record.Usage.Output
		usageSeconds += record.Latency.Seconds()
	}
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
	quantiles, err := exactQuantiles(ctx, w, branch, performance.Latency.Count, ttftCount, latency, ttft)
	if err != nil {
		return nil, performance, nil, err
	}
	performance.Latency = percentileSummary(performance.Latency.Count, latencySum, quantiles[0])
	if ttftCount != 0 {
		p := percentileSummary(ttftCount, ttftSum, quantiles[1])
		performance.TTFT = &p
	}
	if usageSeconds > 0 {
		performance.OutputTPS = float64(performance.OutputTokens) / usageSeconds
		performance.TotalTPS = (float64(performance.InputTokens) + float64(performance.OutputTokens)) / usageSeconds
	}
	metrics = append(metrics, performance.metrics()...)
	for counter := range counters {
		tokenizers = append(tokenizers, counter)
	}
	slices.Sort(tokenizers)
	return metrics, performance, tokenizers, nil
}

func percentileSummary(count int, sum float64, values [3]uint64) Percentiles {
	p := Percentiles{Count: count, P50: float64(values[0]) / 1e6, P95: float64(values[1]) / 1e6, P99: float64(values[2]) / 1e6}
	if count != 0 {
		p.Mean = sum / float64(count) / 1e6
	}
	return p
}

func (p PerformanceSummary) metrics() []MetricResult {
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
			slices.Sort(values)
		}
		for q, percentile := range []int{50, 95, 99} {
			rank := int(math.Ceil(float64(percentile) * float64(count) / 100))
			if len(values) == count {
				out[kind][q] = values[rank-1]
			} else {
				targets = append(targets, radixTarget{kind: kind, quantile: q, rank: rank})
			}
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
