package bench

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type branchRunResult[L comparable] struct {
	scored           []ScoredSample[L]
	sampleResults    []SampleResult
	metrics          map[string]float64
	avgScorePositive float64
	avgScoreNegative float64
	duration         time.Duration
	errors           int
}

// evaluateBranch runs a single branch against all samples.
func (r *BenchRunner[L]) evaluateBranch(ctx context.Context, plan ExecutionPlan, b branch[L], samples []Sample[L]) *branchRunResult[L] {
	start := r.cfg.clock.Now()
	concurrency := plan.Concurrency
	n := len(samples)

	scored := make([]ScoredSample[L], n)
	sampleResults := make([]SampleResult, n)
	errCount := 0
	var mu sync.Mutex

	eval := func(i int) {
		s := samples[i]
		sampleStart := r.cfg.clock.Now()

		evalCtx := ctx
		if r.cfg.timeout > 0 {
			var cancel context.CancelFunc
			evalCtx, cancel = context.WithTimeout(ctx, r.cfg.timeout)
			defer cancel()
		}

		pred, err := b.evaluator.Execute(evalCtx, s.Input)
		elapsed := r.cfg.clock.Now().Sub(sampleStart)

		mu.Lock()
		defer mu.Unlock()

		scored[i] = ScoredSample[L]{Sample: s, Prediction: pred}

		sr := SampleResult{
			ID:        s.ID,
			Label:     fmt.Sprintf("%v", s.Label),
			Predicted: fmt.Sprintf("%v", pred.Label),
			Score:     pred.Score,
			Correct:   s.Label == pred.Label,
			Duration:  elapsed,
		}
		if err != nil {
			sr.Error = err.Error()
			errCount++
		}
		sampleResults[i] = sr
	}

	if concurrency <= 1 {
		for i := range samples {
			eval(i)
		}
	} else {
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for i := range samples {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int) {
				defer wg.Done()
				defer func() { <-sem }()
				eval(idx)
			}(i)
		}
		wg.Wait()
	}

	var posSum, negSum float64
	var posCount, negCount int
	for i, ss := range scored {
		if sampleResults[i].Correct {
			posSum += ss.Prediction.Score
			posCount++
		} else {
			negSum += ss.Prediction.Score
			negCount++
		}
	}

	brMetrics := make(map[string]float64)
	for _, m := range r.cfg.metrics {
		mr := m.Compute(scored)
		brMetrics[mr.Name] = mr.Value
	}

	avgPos := 0.0
	if posCount > 0 {
		avgPos = posSum / float64(posCount)
	}
	avgNeg := 0.0
	if negCount > 0 {
		avgNeg = negSum / float64(negCount)
	}

	return &branchRunResult[L]{
		scored:           scored,
		sampleResults:    sampleResults,
		metrics:          brMetrics,
		avgScorePositive: avgPos,
		avgScoreNegative: avgNeg,
		duration:         r.cfg.clock.Now().Sub(start),
		errors:           errCount,
	}
}
