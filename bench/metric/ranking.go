package metric

import (
	"fmt"
	"math"
	"sort"

	"github.com/kbukum/gokit/bench"
)

// NDCG computes Normalized Discounted Cumulative Gain at k.
// k=0 means use all results. Relevance is 1 if predicted label matches actual.
func NDCG[L comparable](k int) Metric[L] {
	return &ndcg[L]{k: k}
}

type ndcg[L comparable] struct {
	k int
}

func (m *ndcg[L]) Name() string {
	if m.k > 0 {
		return fmt.Sprintf("ndcg@%d", m.k)
	}
	return "ndcg"
}

func (m *ndcg[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: m.Name(), Value: 0}
	}

	// Sort by score descending.
	sorted := make([]bench.ScoredSample[L], len(scored))
	copy(sorted, scored)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Prediction.Score > sorted[j].Prediction.Score
	})

	n := len(sorted)
	if m.k > 0 && m.k < n {
		n = m.k
	}

	// Compute relevance: 1 if predicted label matches actual, 0 otherwise.
	relevances := make([]float64, len(sorted))
	for i, s := range sorted {
		if s.Prediction.Label == s.Sample.Label {
			relevances[i] = 1
		}
	}

	// DCG at n.
	dcg := 0.0
	for i := 0; i < n; i++ {
		dcg += relevances[i] / math.Log2(float64(i+2))
	}

	// Ideal DCG: sort relevances descending.
	idealRel := make([]float64, len(relevances))
	copy(idealRel, relevances)
	sort.Float64s(idealRel)
	// Reverse to get descending order.
	for i, j := 0, len(idealRel)-1; i < j; i, j = i+1, j-1 {
		idealRel[i], idealRel[j] = idealRel[j], idealRel[i]
	}

	idealDCG := 0.0
	for i := 0; i < n; i++ {
		idealDCG += idealRel[i] / math.Log2(float64(i+2))
	}

	ndcgVal := safeDivide(dcg, idealDCG)

	return Result{
		Name:  m.Name(),
		Value: ndcgVal,
		Values: map[string]float64{
			"dcg":       dcg,
			"ideal_dcg": idealDCG,
		},
	}
}

// MAP computes Mean Average Precision.
// positiveLabel identifies relevant items.
func MAP[L comparable](positiveLabel L) Metric[L] {
	return &meanAveragePrecision[L]{positive: positiveLabel}
}

type meanAveragePrecision[L comparable] struct {
	positive L
}

func (m *meanAveragePrecision[L]) Name() string { return "map" }

func (m *meanAveragePrecision[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 {
		return Result{Name: "map", Value: 0}
	}

	// Sort by score descending.
	sorted := make([]bench.ScoredSample[L], len(scored))
	copy(sorted, scored)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Prediction.Score > sorted[j].Prediction.Score
	})

	relevant := 0
	sumPrecision := 0.0
	totalRelevant := 0
	for _, s := range sorted {
		if s.Sample.Label == m.positive {
			totalRelevant++
		}
	}

	if totalRelevant == 0 {
		return Result{Name: "map", Value: 0}
	}

	for i, s := range sorted {
		if s.Sample.Label == m.positive {
			relevant++
			sumPrecision += float64(relevant) / float64(i+1)
		}
	}

	return Result{
		Name:  "map",
		Value: sumPrecision / float64(totalRelevant),
	}
}

// PrecisionAtK computes precision at the top k results.
func PrecisionAtK[L comparable](positiveLabel L, k int) Metric[L] {
	return &precisionAtK[L]{positive: positiveLabel, k: k}
}

type precisionAtK[L comparable] struct {
	positive L
	k        int
}

func (m *precisionAtK[L]) Name() string { return fmt.Sprintf("precision@%d", m.k) }

func (m *precisionAtK[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 || m.k <= 0 {
		return Result{Name: m.Name(), Value: 0}
	}

	sorted := make([]bench.ScoredSample[L], len(scored))
	copy(sorted, scored)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Prediction.Score > sorted[j].Prediction.Score
	})

	n := m.k
	if n > len(sorted) {
		n = len(sorted)
	}

	relevant := 0
	for i := 0; i < n; i++ {
		if sorted[i].Sample.Label == m.positive {
			relevant++
		}
	}

	return Result{
		Name:  m.Name(),
		Value: float64(relevant) / float64(n),
	}
}

// RecallAtK computes recall at the top k results.
func RecallAtK[L comparable](positiveLabel L, k int) Metric[L] {
	return &recallAtK[L]{positive: positiveLabel, k: k}
}

type recallAtK[L comparable] struct {
	positive L
	k        int
}

func (m *recallAtK[L]) Name() string { return fmt.Sprintf("recall@%d", m.k) }

func (m *recallAtK[L]) Compute(scored []bench.ScoredSample[L]) Result {
	if len(scored) == 0 || m.k <= 0 {
		return Result{Name: m.Name(), Value: 0}
	}

	sorted := make([]bench.ScoredSample[L], len(scored))
	copy(sorted, scored)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Prediction.Score > sorted[j].Prediction.Score
	})

	totalRelevant := 0
	for _, s := range sorted {
		if s.Sample.Label == m.positive {
			totalRelevant++
		}
	}

	if totalRelevant == 0 {
		return Result{Name: m.Name(), Value: 0}
	}

	n := m.k
	if n > len(sorted) {
		n = len(sorted)
	}

	relevant := 0
	for i := 0; i < n; i++ {
		if sorted[i].Sample.Label == m.positive {
			relevant++
		}
	}

	return Result{
		Name:  m.Name(),
		Value: float64(relevant) / float64(totalRelevant),
	}
}
