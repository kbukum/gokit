package report

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/kbukum/gokit/bench"
)

// Reporter generates formatted output from benchmark results.
type Reporter interface {
	// Name returns the reporter's format name.
	Name() string
	// Generate writes the formatted report to w.
	Generate(ctx context.Context, w io.Writer, input Input) error
}

// Input uses the same persisted summary, optional records and comparison on every surface.
type Input struct {
	Result *bench.RunResult
	Store  *bench.ResultStore
	Diff   *bench.RunDiff
}

func validateInput(ctx context.Context, input Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.Result == nil {
		return fmt.Errorf("report: result is required")
	}
	return nil
}

func preview(ctx context.Context, input Input, limit int) (rows []bench.SampleResult, err error) {
	if input.Store == nil || len(input.Result.RecordSegments) == 0 {
		return nil, nil
	}
	it, err := input.Store.Records(ctx, input.Result.ID, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, it.Close()) }()
	for len(rows) < limit {
		record, ok, err := it.Next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		rows = append(rows, bench.SampleResult{ID: record.ID, Label: record.LabelText(), Predicted: record.PredictedText(), Score: record.Score, Correct: record.Correct, Duration: record.Latency, Error: record.Error})
	}
	return rows, nil
}
