package stats_test

import (
	"math"
	"testing"

	"github.com/kbukum/gokit/bench/stats"
)

func TestBootstrapMeanIsSeededAndBracketsTheMean(t *testing.T) {
	t.Parallel()
	values := []float64{0.2, 0.4, 0.4, 0.6, 0.8, 1}
	a, err := stats.BootstrapMean(values, stats.DefaultBootstrap(7))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := stats.BootstrapMean(values, stats.DefaultBootstrap(7))
	if a != b {
		t.Fatalf("same seed gave %+v and %+v", a, b)
	}
	if math.Abs(a.Estimate-0.5666666666666667) > 1e-12 || a.N != 6 {
		t.Fatalf("estimate %+v", a)
	}
	if !(a.Low < a.Estimate && a.Estimate < a.High) || a.Low < 0.2 || a.High > 1 {
		t.Fatalf("interval %+v does not bracket the mean within the data range", a)
	}
	c, _ := stats.BootstrapMean(values, stats.DefaultBootstrap(8))
	if c == a {
		t.Fatal("a different seed gave the identical interval")
	}
}

func TestBootstrapMeanSingleValueHasNoSpread(t *testing.T) {
	t.Parallel()
	got, err := stats.BootstrapMean([]float64{0.7}, stats.DefaultBootstrap(1))
	if err != nil || got != (stats.Interval{Estimate: 0.7, Low: 0.7, High: 0.7, N: 1}) || got.Width() != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestBootstrapRefusesBadInput(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		values []float64
		b      stats.Bootstrap
	}{
		"empty":       {nil, stats.DefaultBootstrap(1)},
		"nan":         {[]float64{math.NaN()}, stats.DefaultBootstrap(1)},
		"inf":         {[]float64{1, math.Inf(1)}, stats.DefaultBootstrap(1)},
		"resamples":   {[]float64{1}, stats.Bootstrap{Resamples: 0, Confidence: 0.9}},
		"too many":    {[]float64{1}, stats.Bootstrap{Resamples: stats.MaxResamples + 1, Confidence: 0.9}},
		"confidence":  {[]float64{1}, stats.Bootstrap{Resamples: 10, Confidence: 1}},
		"confidence0": {[]float64{1}, stats.Bootstrap{Resamples: 10}},
	} {
		if _, err := stats.BootstrapMean(tc.values, tc.b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestBradleyTerryRanksByWinsWithSeededIntervals(t *testing.T) {
	t.Parallel()
	var pairs []stats.Pair
	add := func(a, b string, o stats.Outcome, n int) {
		for range n {
			pairs = append(pairs, stats.Pair{A: a, B: b, Outcome: o})
		}
	}
	add("x", "y", stats.AWins, 8)
	add("x", "y", stats.BWins, 2)
	add("y", "z", stats.AWins, 7)
	add("y", "z", stats.Tie, 2)
	add("x", "z", stats.AWins, 5)
	ranks, err := stats.BradleyTerry(pairs, stats.DefaultBootstrap(3))
	if err != nil {
		t.Fatal(err)
	}
	if ranks[0].Item != "x" || ranks[1].Item != "y" || ranks[2].Item != "z" {
		t.Fatalf("order %v", ranks)
	}
	var sum float64
	for _, r := range ranks {
		sum += r.Score.Estimate
		if !(r.Score.Low <= r.Score.Estimate && r.Score.Estimate <= r.Score.High) {
			t.Errorf("%s interval %+v does not hold its estimate", r.Item, r.Score)
		}
	}
	if math.Abs(sum) > 1e-9 {
		t.Fatalf("scores are not centered: sum %v", sum)
	}
	if ranks[0].Comparisons != 15 {
		t.Fatalf("x comparisons %d", ranks[0].Comparisons)
	}
	again, _ := stats.BradleyTerry(pairs, stats.DefaultBootstrap(3))
	for i := range ranks {
		if ranks[i] != again[i] {
			t.Fatalf("same seed differs at %d: %+v vs %+v", i, ranks[i], again[i])
		}
	}
}

func TestBradleyTerryAllTiesAreEqualAndUnbeatenIsFinite(t *testing.T) {
	t.Parallel()
	ties, err := stats.BradleyTerry([]stats.Pair{{A: "a", B: "b", Outcome: stats.Tie}}, stats.DefaultBootstrap(1))
	if err != nil || math.Abs(ties[0].Score.Estimate-ties[1].Score.Estimate) > 1e-9 {
		t.Fatalf("ties %+v, %v", ties, err)
	}
	sweep, err := stats.BradleyTerry([]stats.Pair{{A: "a", B: "b", Outcome: stats.AWins}, {A: "a", B: "b", Outcome: stats.AWins}}, stats.DefaultBootstrap(1))
	if err != nil || math.IsInf(sweep[0].Score.Estimate, 0) || sweep[0].Item != "a" {
		t.Fatalf("sweep %+v, %v", sweep, err)
	}
}

func TestBradleyTerryRefusesBadPairs(t *testing.T) {
	t.Parallel()
	for name, pairs := range map[string][]stats.Pair{
		"empty":   nil,
		"self":    {{A: "a", B: "a", Outcome: stats.AWins}},
		"unnamed": {{A: "", B: "a", Outcome: stats.AWins}},
		"outcome": {{A: "a", B: "b", Outcome: "maybe"}},
	} {
		if _, err := stats.BradleyTerry(pairs, stats.DefaultBootstrap(1)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
