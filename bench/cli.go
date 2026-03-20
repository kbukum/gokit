package bench

import (
	"context"
	"fmt"
	"io"
	"os"
)

// CLIRunner provides CLI-friendly helpers for benchmark operations.
type CLIRunner struct {
	storage RunStorage
	out     io.Writer
}

// CLIOption configures a CLIRunner.
type CLIOption func(*CLIRunner)

// WithOutput sets the output writer for CLI output (default: os.Stdout).
func WithOutput(w io.Writer) CLIOption {
	return func(c *CLIRunner) { c.out = w }
}

// NewCLIRunner creates a CLI runner backed by the given storage.
func NewCLIRunner(storage RunStorage, opts ...CLIOption) *CLIRunner {
	c := &CLIRunner{
		storage: storage,
		out:     os.Stdout,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// RunAndPrint executes a benchmark run and prints the results.
func (c *CLIRunner) RunAndPrint(ctx context.Context, runner *BenchRunner[string], dataset *DatasetLoader[string]) error {
	result, err := runner.Run(ctx, dataset)
	if err != nil {
		return fmt.Errorf("bench cli: run failed: %w", err)
	}
	c.printResult(result)
	return nil
}

// CompareRuns loads two runs by ID and prints their comparison.
func (c *CLIRunner) CompareRuns(ctx context.Context, baseID, targetID string) error {
	base, err := c.storage.Load(ctx, baseID)
	if err != nil {
		return fmt.Errorf("bench cli: load base run: %w", err)
	}
	target, err := c.storage.Load(ctx, targetID)
	if err != nil {
		return fmt.Errorf("bench cli: load target run: %w", err)
	}
	cmp := NewRunComparator()
	diff := cmp.Compare(base, target)

	fmt.Fprintf(c.out, "Comparing: %s → %s\n\n", diff.BaseID, diff.TargetID)
	fmt.Fprint(c.out, diff.Summary())
	return nil
}

// CompareLatest compares the two most recent runs.
func (c *CLIRunner) CompareLatest(ctx context.Context) error {
	summaries, err := c.storage.List(ctx, WithLimit(2))
	if err != nil {
		return fmt.Errorf("bench cli: list runs: %w", err)
	}
	if len(summaries) < 2 {
		return fmt.Errorf("bench cli: need at least 2 runs to compare, found %d", len(summaries))
	}
	return c.CompareRuns(ctx, summaries[1].ID, summaries[0].ID)
}

// ListRuns prints a table of stored runs.
func (c *CLIRunner) ListRuns(ctx context.Context, opts ...ListOption) error {
	summaries, err := c.storage.List(ctx, opts...)
	if err != nil {
		return fmt.Errorf("bench cli: list runs: %w", err)
	}
	if len(summaries) == 0 {
		fmt.Fprintln(c.out, "No runs found.")
		return nil
	}

	fmt.Fprintf(c.out, "%-36s  %-20s  %-16s  %s\n", "ID", "Timestamp", "Dataset", "F1")
	fmt.Fprintf(c.out, "%-36s  %-20s  %-16s  %s\n", "----", "---------", "-------", "--")
	for _, s := range summaries {
		tag := ""
		if s.Tag != "" {
			tag = fmt.Sprintf(" [%s]", s.Tag)
		}
		fmt.Fprintf(c.out, "%-36s  %-20s  %-16s  %.4f%s\n",
			s.ID, s.Timestamp.Format("2006-01-02 15:04:05"), s.Dataset, s.F1, tag)
	}
	return nil
}

// ShowRun loads and prints a specific run in detail.
func (c *CLIRunner) ShowRun(ctx context.Context, runID string) error {
	result, err := c.storage.Load(ctx, runID)
	if err != nil {
		return fmt.Errorf("bench cli: load run: %w", err)
	}
	c.printResult(result)
	return nil
}

func (c *CLIRunner) printResult(r *RunResult) {
	fmt.Fprintf(c.out, "Run: %s\n", r.ID)
	fmt.Fprintf(c.out, "Timestamp: %s\n", r.Timestamp.Format("2006-01-02 15:04:05"))
	if r.Tag != "" {
		fmt.Fprintf(c.out, "Tag: %s\n", r.Tag)
	}
	fmt.Fprintf(c.out, "Dataset: %s (v%s) — %d samples\n", r.Dataset.Name, r.Dataset.Version, r.Dataset.SampleCount)
	fmt.Fprintf(c.out, "Duration: %s\n\n", r.Duration)

	if len(r.Metrics) > 0 {
		fmt.Fprintln(c.out, "Metrics:")
		for _, m := range r.Metrics {
			fmt.Fprintf(c.out, "  %s: %.4f\n", m.Name, m.Value)
			for k, v := range m.Values {
				fmt.Fprintf(c.out, "    %s: %.4f\n", k, v)
			}
		}
		fmt.Fprintln(c.out)
	}

	if len(r.Branches) > 0 {
		fmt.Fprintln(c.out, "Branches:")
		for name, br := range r.Branches {
			fmt.Fprintf(c.out, "  %s (tier %d): %s, errors=%d\n", name, br.Tier, br.Duration, br.Errors)
		}
		fmt.Fprintln(c.out)
	}

	// Sample summary.
	correct := 0
	for _, s := range r.Samples {
		if s.Correct {
			correct++
		}
	}
	fmt.Fprintf(c.out, "Samples: %d/%d correct (%.1f%%)\n", correct, len(r.Samples),
		100*float64(correct)/max(float64(len(r.Samples)), 1))
}
