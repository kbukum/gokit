package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/version"
)

// BenchRunner executes bounded evaluations and commits exact persisted results.
type BenchRunner[L comparable] struct {
	cfg      runConfig[L]
	branches []branch[L]
}

type branch[L comparable] struct {
	name      string
	evaluator Evaluator[L]
	tier      int
	subject   SubjectIdentity
}

type (
	// BranchOption configures one registered evaluator.
	BranchOption func(*branchConfig)
	branchConfig struct {
		tier    int
		subject SubjectIdentity
	}
)

// WithTier sets the branch's display tier.
func WithTier(tier int) BranchOption { return func(c *branchConfig) { c.tier = tier } }

// WithSubject identifies the model and runtime configuration under evaluation.
func WithSubject(subject SubjectIdentity) BranchOption {
	return func(c *branchConfig) { c.subject = subject }
}

// NewBenchRunner configures a runner; Run validates its required store and branches.
func NewBenchRunner[L comparable](opts ...RunOption[L]) *BenchRunner[L] {
	cfg := defaultConfig[L]()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &BenchRunner[L]{cfg: cfg}
}

// Register appends a branch in evaluation order. Configure before calling Run.
func (r *BenchRunner[L]) Register(name string, evaluator Evaluator[L], opts ...BranchOption) {
	var cfg branchConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	r.branches = append(r.branches, branch[L]{name: name, evaluator: evaluator, tier: cfg.tier, subject: cfg.subject})
}

func (r *BenchRunner[L]) validate() error {
	if err := r.cfg.limits.validate(); err != nil {
		return err
	}
	if r.cfg.store == nil {
		return fmt.Errorf("WithStore is required")
	}
	if len(r.branches) == 0 || len(r.branches) > r.cfg.limits.MaxBranches ||
		r.cfg.concurrency < 1 || r.cfg.concurrency > r.cfg.limits.MaxConcurrency ||
		r.cfg.repeats < 1 || r.cfg.repeats > r.cfg.limits.MaxRepeats ||
		r.cfg.warmup < 0 || r.cfg.warmup > r.cfg.limits.MaxWarmup ||
		r.cfg.percentileThreshold < 0 || r.cfg.percentileThreshold > 4096 {
		return fmt.Errorf("execution settings exceed declared limits")
	}
	names := make(map[string]bool)
	for _, b := range r.branches {
		if b.name == "" || names[b.name] || util.IsNil(b.evaluator) {
			return fmt.Errorf("branches must have unique nonempty names and evaluators")
		}
		names[b.name] = true
	}
	names = make(map[string]bool)
	for _, m := range r.cfg.metrics {
		if util.IsNil(m) {
			return fmt.Errorf("nil metric")
		}
	}
	for _, m := range r.cfg.contextMetrics {
		if util.IsNil(m) {
			return fmt.Errorf("nil context metric")
		}
	}
	for _, m := range r.metricIdentities() {
		if m.Name == "" || m.Version == "" || names[m.Name] {
			return fmt.Errorf("metrics must have unique names and algorithm versions")
		}
		names[m.Name] = true
	}
	return nil
}

// Run returns only fully committed results. A run failure rolls back owned objects.
func (r *BenchRunner[L]) Run(parent context.Context, dataset Dataset[L]) (result *RunResult, resultErr error) {
	local := *r
	r = &local
	if r.cfg.timeout > 0 {
		r.cfg.limits.SampleTimeout = r.cfg.timeout
	}
	if err := r.validate(); err != nil {
		return nil, &RunError{Outcome: OutcomeLimitExceeded, Cause: err}
	}
	if util.IsNil(dataset) {
		return nil, &RunError{Outcome: OutcomeDatasetFailed, Cause: fmt.Errorf("dataset is required")}
	}
	ctx, cancel := context.WithTimeout(parent, r.cfg.limits.RunBudget)
	defer cancel()
	start := r.cfg.clock.Now()
	w, beginErr := r.cfg.store.begin(ctx, r.generateID(), len(r.branches), r.cfg.limits)
	if beginErr != nil {
		if parent.Err() != nil {
			return nil, &RunError{Outcome: OutcomeCancelled, Cause: parent.Err()}
		}
		return nil, &RunError{Outcome: OutcomeStorageFailed, Cause: beginErr}
	}
	defer w.release()
	defer func() {
		if resultErr == nil {
			return
		}
		cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), r.cfg.limits.CleanupTimeout)
		defer stop()
		cleanupErr := r.cfg.store.Delete(cleanup, w.id)
		var runErr *RunError
		if !errors.As(resultErr, &runErr) {
			resultErr = &RunError{Outcome: OutcomeDatasetFailed, Cause: resultErr}
		}
		if parent.Err() != nil {
			resultErr = &RunError{Outcome: OutcomeCancelled, Cause: errors.Join(parent.Err(), resultErr)}
		} else if ctx.Err() != nil {
			resultErr = &RunError{Outcome: OutcomeBudgetExceeded, Cause: errors.Join(ctx.Err(), resultErr)}
		}
		resultErr = errors.Join(resultErr, cleanupErr)
		result = nil
	}()
	descriptor, describeErr := dataset.Describe(ctx)
	if describeErr != nil {
		return nil, describeErr
	}
	result = &RunResult{ID: w.id, Timestamp: start, Tag: r.cfg.tag, Branches: make(map[string]BranchResult)}
	tokenizers := make(map[string]bool)
	for idx, b := range r.branches {
		warmupFailures, err := r.warmup(ctx, dataset, b)
		if err != nil {
			return nil, err
		}
		info, digest, err := r.dispatch(ctx, dataset, b, idx, w)
		if err != nil {
			return nil, err
		}
		if idx == 0 {
			result.Dataset = info
			result.Dataset.Name, result.Dataset.Version = descriptor.Name, descriptor.Version
			result.Provenance.DatasetHash = digest
		} else if digest != result.Provenance.DatasetHash {
			return nil, fmt.Errorf("dataset changed between branches")
		}
		metrics, performance, counters, err := r.measure(ctx, w, idx)
		if err != nil {
			return nil, err
		}
		for _, counter := range counters {
			tokenizers[counter] = true
		}
		performance.WarmupFailures = warmupFailures
		subjectHash, err := fingerprint(b.subject)
		if err != nil {
			return nil, err
		}
		br := BranchResult{Name: b.name, Tier: b.tier, Performance: performance, Subject: b.subject, SubjectFingerprint: subjectHash, Metrics: make(map[string]float64), Errors: performance.Failures}
		for metricIndex := range metrics {
			m := &metrics[metricIndex]
			br.Metrics[m.Name] = m.Value
		}
		br.MetricResults = metrics
		result.Branches[b.name] = br
		if idx == 0 {
			result.Metrics = metrics
			result.ScoreDistributions, result.DistributionsOmitted, err = distributions(ctx, w, idx)
			if err != nil {
				return nil, err
			}
		}
	}
	result.Duration = r.cfg.clock.Now().Sub(start)
	r.provenance(result)
	datasetIdentity, err := fingerprint(struct {
		DatasetDescriptor
		Content string `json:"content"`
	}{descriptor, result.Provenance.DatasetHash})
	if err != nil {
		return nil, err
	}
	result.Evaluation = EvaluationIdentity{
		Version: "1", Dataset: datasetIdentity, Metrics: r.metricIdentities(),
		Judges:    result.Provenance.Judges,
		Execution: ExecutionIdentity{Concurrency: r.cfg.concurrency, Repeats: r.cfg.repeats, Warmup: r.cfg.warmup, SampleTimeout: r.cfg.limits.SampleTimeout, Seed: r.cfg.seed},
	}
	for tokenizer := range tokenizers {
		result.Evaluation.Tokenizers = append(result.Evaluation.Tokenizers, tokenizer)
	}
	slices.Sort(result.Evaluation.Tokenizers)
	evaluationFingerprint, fingerprintErr := fingerprint(result.Evaluation)
	if fingerprintErr != nil {
		return nil, fingerprintErr
	}
	result.EvaluationFingerprint = evaluationFingerprint
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := w.commit(ctx, result); err != nil {
		var runErr *RunError
		if errors.As(err, &runErr) {
			return nil, err
		}
		return nil, &RunError{Outcome: OutcomeStorageFailed, Cause: err}
	}
	return result, nil
}

func (r *BenchRunner[L]) metricIdentities() []MetricIdentity {
	out := make([]MetricIdentity, 0, len(r.cfg.metrics)+len(r.cfg.contextMetrics))
	for _, m := range r.cfg.metrics {
		out = append(out, m.Identity())
	}
	for _, m := range r.cfg.contextMetrics {
		out = append(out, m.Identity())
	}
	for _, name := range []string{"latency_ms", "ttft_ms", "throughput_output_tps", "throughput_total_tps", "failure_rate", "usage_tokens"} {
		out = append(out, MetricIdentity{Name: name, Version: "1"})
	}
	return out
}

func (r *BenchRunner[L]) provenance(result *RunResult) {
	p := &result.Provenance
	p.Seed, p.RNGAlgorithm = r.cfg.seed, RNGAlgorithm
	p.GitCommit, p.GitTreeState = r.cfg.probe.GitCommit(), r.cfg.probe.GitTreeState()
	p.Host, p.OS, p.Arch = r.cfg.probe.Host(), r.cfg.probe.OS(), r.cfg.probe.Arch()
	p.ToolVersion = version.GetShortVersion()
	p.DatasetName, p.DatasetVersion = result.Dataset.Name, result.Dataset.Version
	for _, b := range r.branches {
		p.Branches = append(p.Branches, b.name)
		judges := judgeProvenance(result.Branches[b.name].MetricResults)
		for i := range judges {
			judges[i].Branch = b.name
		}
		p.Judges = append(p.Judges, judges...)
	}
	for metricIndex := range result.Metrics {
		m := &result.Metrics[metricIndex]
		p.Metrics = append(p.Metrics, m.Name)
	}
}

func hashSample[L comparable](h *util.ContentHasher, sample Sample[L], label []byte) error {
	metadata, err := json.Marshal(sample.Metadata)
	if err != nil {
		return fmt.Errorf("bench: encode sample metadata: %w", err)
	}
	h.UpdateFramed([]byte("id"), []byte(sample.ID))
	h.UpdateFramed([]byte("input"), sample.Input)
	h.UpdateFramed([]byte("label"), label)
	h.UpdateFramed([]byte("source"), []byte(sample.Source))
	h.UpdateFramed([]byte("metadata"), metadata)
	return nil
}

func finishDatasetHash(h *util.ContentHasher, count int) string {
	h.UpdateFramed([]byte("count"), []byte(strconv.Itoa(count)))
	return h.FinalizeHex()
}
