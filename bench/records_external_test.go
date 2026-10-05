package bench_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func externalSampleResults(t *testing.T, store *bench.ResultStore, result *bench.RunResult) []bench.SampleResult {
	t.Helper()
	iter, err := store.Records(context.Background(), result.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := iter.Close(); err != nil {
			t.Error(err)
		}
	}()
	var samples []bench.SampleResult
	for {
		r, ok, err := iter.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		samples = append(samples, bench.SampleResult{ID: r.ID, Label: r.LabelText(), Predicted: r.PredictedText(), Correct: r.Correct, Duration: r.Latency, Score: r.Score, Error: r.Error})
	}
	return samples
}
