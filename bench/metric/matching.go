package metric

import "github.com/kbukum/gokit/bench"

// ExactMatch computes the fraction of exact label matches.
func ExactMatch[L comparable]() Metric[L] {
	return &exactMatch[L]{}
}

type exactMatch[L comparable] struct{}

func (m *exactMatch[L]) Name() string { return "exact_match" }

func (m *exactMatch[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "exact_match", Value: 0}
	}

	correct := 0
	for _, s := range scored {
		if s.Prediction.Label == s.Sample.Label {
			correct++
		}
	}

	return Result{
		Name:  "exact_match",
		Value: float64(correct) / float64(len(scored)),
	}
}

// FuzzyMatch computes string similarity using a Levenshtein distance ratio.
// threshold is the minimum similarity (0-1) to count as a match.
func FuzzyMatch(threshold float64) Metric[string] {
	return &fuzzyMatch{threshold: threshold}
}

type fuzzyMatch struct {
	threshold float64
}

func (m *fuzzyMatch) Name() string { return "fuzzy_match" }

func (m *fuzzyMatch) Compute(scored []bench.ScoredSample[string]) Result {
	if len(scored) == 0 {
		return Result{Name: "fuzzy_match", Value: 0}
	}

	matches := 0
	sumSimilarity := 0.0
	for _, s := range scored {
		sim := levenshteinSimilarity(s.Sample.Label, s.Prediction.Label)
		sumSimilarity += sim
		if sim >= m.threshold {
			matches++
		}
	}

	return Result{
		Name:  "fuzzy_match",
		Value: float64(matches) / float64(len(scored)),
		Values: map[string]float64{
			"mean_similarity": sumSimilarity / float64(len(scored)),
			"threshold":       m.threshold,
		},
	}
}

// levenshteinSimilarity returns a similarity ratio in [0, 1] based on
// the Levenshtein edit distance between two strings.
func levenshteinSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	if maxLen == 0 {
		return 1
	}

	// Standard dynamic-programming Levenshtein distance.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = ins
			if del < curr[j] {
				curr[j] = del
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}

	dist := prev[lb]
	return 1 - float64(dist)/float64(maxLen)
}
