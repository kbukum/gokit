package bench

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/process"
)

// processEvaluator wraps a subprocess call as an Evaluator.
type processEvaluator[L comparable] struct {
	name        string
	buildCmd    func(Sample[L]) process.Command
	parseOutput func(*process.Result) (Prediction[L], error)
}

// FromProcess creates an Evaluator that calls a subprocess.
// buildCmd creates the process command from a sample's raw input.
// parseOutput extracts a prediction from the process result.
func FromProcess[L comparable](
	name string,
	buildCmd func(Sample[L]) process.Command,
	parseOutput func(*process.Result) (Prediction[L], error),
) Evaluator[L] {
	return &processEvaluator[L]{
		name:        name,
		buildCmd:    buildCmd,
		parseOutput: parseOutput,
	}
}

func (e *processEvaluator[L]) Name() string                       { return e.name }
func (e *processEvaluator[L]) IsAvailable(_ context.Context) bool { return true }

func (e *processEvaluator[L]) Execute(ctx context.Context, input []byte) (Prediction[L], error) {
	// Build a sample with only input populated; buildCmd can use the raw bytes.
	sample := Sample[L]{Input: input}
	cmd := e.buildCmd(sample)

	result, err := process.Run(ctx, cmd)
	if err != nil {
		var zero Prediction[L]
		return zero, fmt.Errorf("bench: process %s: %w", e.name, err)
	}

	pred, err := e.parseOutput(result)
	if err != nil {
		var zero Prediction[L]
		return zero, fmt.Errorf("bench: parse output %s: %w", e.name, err)
	}
	return pred, nil
}
