package bench_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
)

func TestEvaluationDatasetIdentity(t *testing.T) {
	t.Parallel()
	run := func(descriptor bench.DatasetDescriptor) *bench.RunResult {
		t.Helper()
		store := bench.NewResultStore(bench.NewDirStore(t.TempDir()))
		runner := bench.NewBenchRunner(bench.WithStore[string](store), bench.WithMetrics[string](metric.ExactMatch[string]()))
		evaluator := bench.EvaluatorFunc("echo", func(context.Context, []byte) (bench.Prediction[string], error) {
			return bench.Prediction[string]{Label: "yes"}, nil
		})
		runner.Register("first", evaluator)
		runner.Register("second", evaluator)
		result, err := runner.Run(t.Context(), bench.NewSliceDataset(descriptor, []bench.Sample[string]{{ID: "a", Label: "yes"}}))
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := store.Load(t.Context(), result.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.Branches["second"].MetricResults) == 0 || !reflect.DeepEqual(result.Branches["second"].MetricResults, loaded.Branches["second"].MetricResults) {
			t.Fatal("secondary branch lost typed metrics")
		}
		return result
	}
	base := run(bench.DatasetDescriptor{Name: "dataset", Version: "1"})
	for _, tc := range []struct {
		name       string
		descriptor bench.DatasetDescriptor
		eligible   bool
	}{
		{"same", bench.DatasetDescriptor{Name: "dataset", Version: "1"}, true},
		{"name", bench.DatasetDescriptor{Name: "other", Version: "1"}, false},
		{"version", bench.DatasetDescriptor{Name: "dataset", Version: "2"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			target := run(tc.descriptor)
			diff := bench.NewRunComparator().Compare(base, target)
			if diff.Eligibility.Eligible != tc.eligible {
				t.Fatalf("eligibility: %+v", diff.Eligibility)
			}
			if base.Provenance.DatasetHash != target.Provenance.DatasetHash {
				t.Fatal("identical records must have identical content hashes")
			}
		})
	}
}
