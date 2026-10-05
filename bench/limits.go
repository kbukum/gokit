package bench

import (
	"fmt"
	"time"
)

// Outcome classifies terminal run failures. Sample failures are persisted records.
type Outcome string

const (
	OutcomeCancelled      Outcome = "canceled"
	OutcomeBudgetExceeded Outcome = "budget_exceeded"
	OutcomeLimitExceeded  Outcome = "limit_exceeded"
	OutcomeStorageFailed  Outcome = "storage_failed"
	OutcomeDatasetFailed  Outcome = "dataset_failed"
	OutcomeMetricFailed   Outcome = "metric_failed"
)

// RunError preserves both the terminal outcome and its cause.
type RunError struct {
	Outcome Outcome
	Cause   error
}

func (e *RunError) Error() string { return fmt.Sprintf("bench: %s: %v", e.Outcome, e.Cause) }
func (e *RunError) Unwrap() error { return e.Cause }

// Limits bounds admission, retained state, object sizes, and resource lifetimes.
type Limits struct {
	MaxSamples         int
	MaxBranches        int
	MaxRepeats         int
	MaxWarmup          int
	MaxConcurrency     int
	MaxRecordBytes     int
	MaxRetainedSamples int
	MaxLabels          int
	SegmentBytes       int
	MaxRunBytes        int64
	MaxSummaryBytes    int64
	SampleTimeout      time.Duration
	RunBudget          time.Duration
	CleanupTimeout     time.Duration
}

// DefaultLimits returns finite production defaults.
func DefaultLimits() Limits {
	return Limits{
		MaxSamples: 100_000, MaxBranches: 16, MaxRepeats: 100, MaxWarmup: 100,
		MaxConcurrency: 64, MaxRecordBytes: 1 << 20, MaxRetainedSamples: 100_000,
		MaxLabels: 1024, SegmentBytes: 256 << 10, MaxRunBytes: 256 << 20,
		MaxSummaryBytes: 16 << 20, SampleTimeout: 5 * time.Minute,
		RunBudget: 2 * time.Hour, CleanupTimeout: 30 * time.Second,
	}
}

func (l Limits) validate() error {
	if l.MaxSamples < 1 || l.MaxBranches < 1 || l.MaxRepeats < 1 || l.MaxWarmup < 0 ||
		l.MaxConcurrency < 1 || l.MaxRecordBytes < 1 || l.MaxRetainedSamples < 1 ||
		l.MaxLabels < 1 || l.SegmentBytes < 1 || l.MaxRunBytes < 1 ||
		l.MaxSummaryBytes < 1 || l.SampleTimeout <= 0 || l.RunBudget <= 0 || l.CleanupTimeout <= 0 {
		return fmt.Errorf("all limits must be positive (warmup may be zero)")
	}
	return nil
}
