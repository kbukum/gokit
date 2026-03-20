package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BenchRunner orchestrates evaluation runs.
type BenchRunner[L comparable] struct {
	cfg      runConfig[L]
	branches []branch[L]
}

type branch[L comparable] struct {
	name      string
	evaluator Evaluator[L]
	tier      int
}

// BranchOption configures a branch registration.
type BranchOption func(*branchConfig)

type branchConfig struct {
	tier int
}

// WithTier sets the tier for a branch (used for tiered evaluation).
func WithTier(tier int) BranchOption {
	return func(c *branchConfig) { c.tier = tier }
}

// NewBenchRunner creates a new runner with the given options.
func NewBenchRunner[L comparable](opts ...RunOption[L]) *BenchRunner[L] {
	cfg := defaultConfig[L]()
	for _, o := range opts {
		o(&cfg)
	}
	return &BenchRunner[L]{cfg: cfg}
}

// Register adds an evaluator branch to the runner.
func (r *BenchRunner[L]) Register(name string, eval Evaluator[L], opts ...BranchOption) {
	bc := branchConfig{}
	for _, o := range opts {
		o(&bc)
	}
	r.branches = append(r.branches, branch[L]{
		name:      name,
		evaluator: eval,
		tier:      bc.tier,
	})
}

// Run executes the benchmark: loads samples, runs evaluators, computes metrics, and stores results.
func (r *BenchRunner[L]) Run(ctx context.Context, dataset *DatasetLoader[L]) (*RunResult, error) {
	start := time.Now()

	// Load all samples.
	samples, err := dataset.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("bench: load dataset: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("bench: dataset is empty")
	}

	// Load manifest for dataset info.
	manifest, err := dataset.Manifest()
	if err != nil {
		return nil, fmt.Errorf("bench: load manifest: %w", err)
	}

	// Build label distribution.
	labelDist := make(map[string]int)
	for _, s := range samples {
		labelDist[fmt.Sprintf("%v", s.Label)]++
	}

	// Use first branch if only one, otherwise evaluate all branches.
	if len(r.branches) == 0 {
		return nil, fmt.Errorf("bench: no evaluator branches registered")
	}

	// Evaluate all branches and collect per-sample results.
	branchResults := make(map[string]*branchRunResult[L])
	sampleResults := make([]SampleResult, len(samples))

	// Initialize sample results.
	for i, s := range samples {
		sampleResults[i] = SampleResult{
			ID:           s.ID,
			Label:        fmt.Sprintf("%v", s.Label),
			BranchScores: make(map[string]float64),
		}
	}

	for _, b := range r.branches {
		br := r.evaluateBranch(ctx, b, samples)
		branchResults[b.name] = br

		// Merge per-sample info from the first branch (primary).
		for i, sr := range br.sampleResults {
			sampleResults[i].BranchScores[b.name] = sr.Score
			// Use first branch's prediction as the primary result.
			if b == r.branches[0] {
				sampleResults[i].Predicted = sr.Predicted
				sampleResults[i].Score = sr.Score
				sampleResults[i].Correct = sr.Correct
				sampleResults[i].Duration = sr.Duration
				sampleResults[i].Error = sr.Error
			}
		}
	}

	// Build ScoredSamples from the primary branch for metrics.
	primaryBranch := r.branches[0]
	scored := branchResults[primaryBranch.name].scored

	// Compute metrics.
	metrics := make([]MetricResult, 0, len(r.cfg.metrics))
	for _, m := range r.cfg.metrics {
		metrics = append(metrics, m.Compute(scored))
	}

	// Build branch result map.
	branches := make(map[string]BranchResult, len(r.branches))
	for _, b := range r.branches {
		br := branchResults[b.name]
		branches[b.name] = BranchResult{
			Name:             b.name,
			Tier:             b.tier,
			Metrics:          br.metrics,
			AvgScorePositive: br.avgScorePositive,
			AvgScoreNegative: br.avgScoreNegative,
			Duration:         br.duration,
			Errors:           br.errors,
		}
	}

	// Generate run ID.
	runID := r.generateID()

	result := &RunResult{
		ID:        runID,
		Schema:    SchemaVersion,
		Timestamp: start,
		Tag:       r.cfg.tag,
		Duration:  time.Since(start),
		Dataset: DatasetInfo{
			Name:              manifest.Name,
			Version:           manifest.Version,
			SampleCount:       len(samples),
			LabelDistribution: labelDist,
		},
		Metrics:  metrics,
		Branches: branches,
		Samples:  sampleResults,
	}

	// Store if configured.
	if r.cfg.storage != nil {
		if _, err := r.cfg.storage.Save(ctx, result); err != nil {
			return result, fmt.Errorf("bench: save result: %w", err)
		}
	}

	return result, nil
}

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
func (r *BenchRunner[L]) evaluateBranch(ctx context.Context, b branch[L], samples []Sample[L]) *branchRunResult[L] {
	start := time.Now()
	n := len(samples)

	scored := make([]ScoredSample[L], n)
	sampleResults := make([]SampleResult, n)
	errCount := 0
	var mu sync.Mutex

	eval := func(i int) {
		s := samples[i]
		sampleStart := time.Now()

		evalCtx := ctx
		if r.cfg.timeout > 0 {
			var cancel context.CancelFunc
			evalCtx, cancel = context.WithTimeout(ctx, r.cfg.timeout)
			defer cancel()
		}

		pred, err := b.evaluator.Execute(evalCtx, s.Input)
		elapsed := time.Since(sampleStart)

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

	if r.cfg.concurrency <= 1 {
		for i := range samples {
			eval(i)
		}
	} else {
		sem := make(chan struct{}, r.cfg.concurrency)
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

	// Compute branch-level score averages.
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
		duration:         time.Since(start),
		errors:           errCount,
	}
}

func (r *BenchRunner[L]) generateID() string {
	ts := time.Now().Format("20060102-150405")
	if r.cfg.tag != "" {
		return fmt.Sprintf("%s_%s", r.cfg.tag, ts)
	}
	return fmt.Sprintf("run_%s_%s", ts, uuid.New().String()[:8])
}
