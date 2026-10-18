package bench_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
)

func TestSummarizePerformance(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	ttft := ms(5)
	measurements := []bench.Measurement{
		{Latency: ms(30), TTFT: &ttft, Usage: bench.TokenUsage{Input: 10, Output: 20, Source: bench.UsageReported}},
		{Latency: ms(10), Usage: bench.TokenUsage{Source: bench.UsageUnavailable}},
		{Latency: ms(20), Usage: bench.TokenUsage{Input: 5, Output: 5, Source: bench.UsageCounted, Tokenizer: "t"}},
		{Latency: ms(999), Usage: bench.TokenUsage{Source: bench.UsageUnavailable}, Failed: true},
	}
	p, err := bench.SummarizePerformance(measurements)
	if err != nil {
		t.Fatal(err)
	}
	if p.Observations != 4 || p.Failures != 1 || p.WithoutUsage != 1 || p.InputTokens != 15 || p.OutputTokens != 25 {
		t.Fatalf("counts: %+v", p)
	}
	if p.Latency.Count != 3 || p.Latency.P50 != 20 || p.Latency.P95 != 30 || p.Latency.P99 != 30 || p.Latency.Mean != 20 {
		t.Fatalf("latency: %+v", p.Latency)
	}
	if p.TTFT == nil || p.TTFT.Count != 1 || p.TTFT.P50 != 5 {
		t.Fatalf("ttft: %+v", p.TTFT)
	}
	if math.Abs(p.OutputTPS-25/0.05) > 1e-9 || math.Abs(p.TotalTPS-40/0.05) > 1e-9 {
		t.Fatalf("throughput: %+v", p)
	}
	metrics := map[string]bench.MetricResult{}
	for _, m := range p.Metrics() {
		metrics[m.Name] = m
	}
	if m := metrics["latency_ms"]; m.Value != 20 || m.Values["p95"] != 30 || m.Direction != bench.LowerIsBetter {
		t.Fatalf("latency_ms: %+v", m)
	}
	if m := metrics["ttft_ms"]; m.Value != 5 {
		t.Fatalf("ttft_ms: %+v", m)
	}
	if m := metrics["usage_tokens"]; m.Value != 40 || m.Values["without_usage"] != 1 || m.Direction != bench.Neutral {
		t.Fatalf("usage_tokens: %+v", m)
	}
	if m := metrics["failure_rate"]; m.Value != .25 {
		t.Fatalf("failure_rate: %+v", m)
	}
}

func TestSummarizePerformanceWithoutTTFTOmitsIt(t *testing.T) {
	p, err := bench.SummarizePerformance([]bench.Measurement{{Latency: time.Millisecond, Usage: bench.TokenUsage{Source: bench.UsageReported}}})
	if err != nil || p.TTFT != nil {
		t.Fatalf("%+v %v", p, err)
	}
	for _, m := range p.Metrics() {
		if m.Name == "ttft_ms" {
			t.Fatal("ttft_ms reported without observations")
		}
	}
	empty, err := bench.SummarizePerformance(nil)
	if err != nil || empty.Observations != 0 || empty.Latency.Count != 0 {
		t.Fatalf("%+v %v", empty, err)
	}
}

func TestSummarizePerformanceRefusesInvalidMeasurements(t *testing.T) {
	negative := -time.Millisecond
	for name, tc := range map[string]struct {
		in   []bench.Measurement
		want string
	}{
		"latency": {[]bench.Measurement{{Latency: -1, Usage: bench.TokenUsage{Source: bench.UsageReported}}}, "negative"},
		"ttft":    {[]bench.Measurement{{TTFT: &negative, Usage: bench.TokenUsage{Source: bench.UsageReported}}}, "negative"},
		"tokens":  {[]bench.Measurement{{Usage: bench.TokenUsage{Input: -1, Source: bench.UsageReported}}}, "negative"},
		"source":  {[]bench.Measurement{{Usage: bench.TokenUsage{Source: "guessed"}}}, "unknown usage source"},
		"overflow": {[]bench.Measurement{
			{Usage: bench.TokenUsage{Input: math.MaxInt64, Source: bench.UsageReported}},
			{Usage: bench.TokenUsage{Input: 1, Source: bench.UsageReported}},
		}, "overflow"},
		"limit": {make([]bench.Measurement, bench.MaxMeasurements+1), "exceed"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := bench.SummarizePerformance(tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
