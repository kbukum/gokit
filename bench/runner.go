package bench

import (
	"context"
	"fmt"
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
	start := r.cfg.clock.Now()
	runID := r.generateID()
	plan := NewExecutionPlan(r.cfg.concurrency)

	samples, err := dataset.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("bench: load dataset: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("bench: dataset is empty")
	}

	manifest, err := dataset.Manifest()
	if err != nil {
		return nil, fmt.Errorf("bench: load manifest: %w", err)
	}

	labelDist := make(map[string]int)
	for _, s := range samples {
		labelDist[fmt.Sprintf("%v", s.Label)]++
	}

	if len(r.branches) == 0 {
		return nil, fmt.Errorf("bench: no evaluator branches registered")
	}

	branchResults := make(map[string]*branchRunResult[L])
	sampleResults := make([]SampleResult, len(samples))

	for i, s := range samples {
		sampleResults[i] = SampleResult{
			ID:           s.ID,
			Label:        fmt.Sprintf("%v", s.Label),
			BranchScores: make(map[string]float64),
		}
	}

	for _, b := range r.branches {
		br := r.evaluateBranch(ctx, plan, b, samples)
		branchResults[b.name] = br

		for i, sr := range br.sampleResults {
			sampleResults[i].BranchScores[b.name] = sr.Score
			if b == r.branches[0] {
				sampleResults[i].Predicted = sr.Predicted
				sampleResults[i].Score = sr.Score
				sampleResults[i].Correct = sr.Correct
				sampleResults[i].Duration = sr.Duration
				sampleResults[i].Error = sr.Error
			}
		}
	}

	primaryBranch := r.branches[0]
	scored := branchResults[primaryBranch.name].scored

	metrics := make([]MetricResult, 0, len(r.cfg.metrics))
	for _, m := range r.cfg.metrics {
		metrics = append(metrics, m.Compute(scored))
	}

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

	result := &RunResult{
		ID:        runID,
		Schema:    SchemaVersion,
		Timestamp: start,
		Tag:       r.cfg.tag,
		Duration:  r.cfg.clock.Now().Sub(start),
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

	if r.cfg.storage != nil {
		if _, err := r.cfg.storage.Save(ctx, result); err != nil {
			return result, fmt.Errorf("bench: save result: %w", err)
		}
	}

	return result, nil
}
