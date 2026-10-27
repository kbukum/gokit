package stats

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	apperrors "github.com/kbukum/gokit/errors"
)

// Outcome is one pairwise result.
type Outcome string

const (
	AWins Outcome = "a"
	BWins Outcome = "b"
	Tie   Outcome = "tie"
)

// Pair is one comparison between two distinct items.
type Pair struct {
	A, B    string
	Outcome Outcome
}

// Limits on one ranking: items and pairs.
const (
	MaxItems = 256
	MaxPairs = 10_000
)

const (
	btIterations = 500
	btTolerance  = 1e-10
	// btPrior adds one virtual tie between each item and a fixed reference of strength 1, so an item that never lost or never won still has a finite strength.
	btPrior = 1.0
)

// Rank is one item's Bradley–Terry score: the log strength, centered so the items average zero, with its bootstrap interval, and the comparisons it took part in.
type Rank struct {
	Item        string
	Score       Interval
	Comparisons int
}

// BradleyTerry fits a Bradley–Terry model to pairs and returns the items ranked by score, best first, with seeded bootstrap intervals over resampled pairs. A tie counts as half a win for each side. It needs 1 to [MaxPairs] pairs over at most [MaxItems] items.
func BradleyTerry(pairs []Pair, b Bootstrap) ([]Rank, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if len(pairs) == 0 || len(pairs) > MaxPairs {
		return nil, apperrors.InvalidInput("pairs", fmt.Sprintf("need 1 to %d pairs", MaxPairs))
	}
	index := map[string]int{}
	var items []string
	counts := []int{}
	for _, p := range pairs {
		if p.A == "" || p.B == "" || p.A == p.B {
			return nil, apperrors.InvalidInput("pairs", "a pair needs two distinct, named items")
		}
		if p.Outcome != AWins && p.Outcome != BWins && p.Outcome != Tie {
			return nil, apperrors.InvalidInput("pairs", fmt.Sprintf("outcome %q is not a, b or tie", p.Outcome))
		}
		for _, it := range []string{p.A, p.B} {
			i, ok := index[it]
			if !ok {
				if len(items) == MaxItems {
					return nil, apperrors.InvalidInput("pairs", fmt.Sprintf("at most %d items", MaxItems))
				}
				i = len(items)
				index[it] = i
				items = append(items, it)
				counts = append(counts, 0)
			}
			counts[i]++
		}
	}
	coded := make([]codedPair, len(pairs))
	for i, p := range pairs {
		coded[i] = codedPair{index[p.A], index[p.B], p.Outcome}
	}
	estimate := fit(coded, len(items))
	r := b.rand()
	samples := make([][]float64, len(items))
	resampled := make([]codedPair, len(coded))
	for range b.Resamples {
		for i := range resampled {
			resampled[i] = coded[r.IntN(len(coded))]
		}
		for i, s := range fit(resampled, len(items)) {
			samples[i] = append(samples[i], s)
		}
	}
	out := make([]Rank, len(items))
	for i, it := range items {
		low, high := percentiles(samples[i], b.Confidence)
		out[i] = Rank{Item: it, Score: Interval{Estimate: estimate[i], Low: low, High: high, N: counts[i]}, Comparisons: counts[i]}
	}
	slices.SortStableFunc(out, func(x, y Rank) int {
		if c := cmp.Compare(y.Score.Estimate, x.Score.Estimate); c != 0 {
			return c
		}
		return cmp.Compare(x.Item, y.Item)
	})
	return out, nil
}

type codedPair struct {
	a, b    int
	outcome Outcome
}

// fit runs Hunter's minorization–maximization updates and returns centered log strengths.
func fit(pairs []codedPair, n int) []float64 {
	wins := make([]float64, n)
	for i := range wins {
		wins[i] = btPrior / 2
	}
	for _, p := range pairs {
		switch p.outcome {
		case AWins:
			wins[p.a]++
		case BWins:
			wins[p.b]++
		case Tie:
			wins[p.a] += 0.5
			wins[p.b] += 0.5
		}
	}
	strength := make([]float64, n)
	for i := range strength {
		strength[i] = 1
	}
	next := make([]float64, n)
	for range btIterations {
		denom := make([]float64, n)
		for i := range denom {
			denom[i] = btPrior / (strength[i] + 1)
		}
		for _, p := range pairs {
			d := 1 / (strength[p.a] + strength[p.b])
			denom[p.a] += d
			denom[p.b] += d
		}
		change := 0.0
		for i := range next {
			next[i] = wins[i] / denom[i]
			change = math.Max(change, math.Abs(math.Log(next[i])-math.Log(strength[i])))
		}
		copy(strength, next)
		if change < btTolerance {
			break
		}
	}
	out := make([]float64, n)
	var sum float64
	for i, s := range strength {
		out[i] = math.Log(s)
		sum += out[i]
	}
	for i := range out {
		out[i] -= sum / float64(n)
	}
	return out
}
