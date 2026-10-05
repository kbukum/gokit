package bench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/bench/report"
	benchtest "github.com/kbukum/gokit/bench/testutil"
	"github.com/kbukum/gokit/stream"
	"github.com/kbukum/gokit/util"
)

type observedFixture struct{ clock *util.FakeClock }

type overflowFixture struct{ observedFixture }

func (e *overflowFixture) ExecuteObserved(_ context.Context, _ []byte, o bench.Observer) (bench.Prediction[string], error) {
	e.clock.Advance(time.Millisecond)
	o.ReportUsage(bench.TokenUsage{Source: bench.UsageReported, Input: math.MaxInt64})
	return bench.Prediction[string]{Label: "yes"}, nil
}

func TestEvaluationUsageOverflow(t *testing.T) {
	t.Parallel()
	objects := bench.NewDirStore(t.TempDir())
	clock := util.NewFakeClock(time.Unix(0, 0))
	runner := bench.NewBenchRunner(bench.WithStore[string](bench.NewResultStore(objects)), bench.WithClock[string](clock))
	runner.Register("overflow", &overflowFixture{observedFixture{clock: clock}})
	result, err := runner.Run(t.Context(), generatedDataset(2))
	var runErr *bench.RunError
	if result != nil || !errors.As(err, &runErr) || runErr.Outcome != bench.OutcomeLimitExceeded {
		t.Fatalf("overflow outcome: %v %v", result, err)
	}
	keys, err := objects.List(t.Context(), "")
	if err != nil || len(keys) != 0 {
		t.Fatalf("overflow cleanup: %v %v", keys, err)
	}
}

func (e *observedFixture) Name() string                     { return "fixture" }
func (e *observedFixture) IsAvailable(context.Context) bool { return true }
func (e *observedFixture) Execute(ctx context.Context, input []byte) (bench.Prediction[string], error) {
	return e.ExecuteObserved(ctx, input, nil)
}

func (e *observedFixture) ExecuteObserved(_ context.Context, input []byte, o bench.Observer) (bench.Prediction[string], error) {
	e.clock.Advance(2 * time.Millisecond)
	if o != nil {
		o.ContentStarted()
		o.ReportUsage(bench.TokenUsage{Source: bench.UsageReported, Input: 3, Output: 2})
	}
	e.clock.Advance(8 * time.Millisecond)
	return bench.Prediction[string]{Label: string(input), Score: .75}, nil
}

func TestConsumerEvaluationPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := bench.NewResultStore(bench.NewDirStore(t.TempDir()))
	dataset := bench.NewSliceDataset(bench.DatasetDescriptor{Name: "consumer", Version: "1"}, []bench.Sample[string]{
		{ID: "a", Input: []byte("the cat is sitting on the mat"), Label: "the cat is sitting on the mat"},
		{ID: "b", Input: []byte("the cat is sitting on the mat"), Label: "the cat is sitting on the mat"},
	})
	run := func(id, subject string, threshold int) *bench.RunResult {
		t.Helper()
		clock := util.NewFakeClock(time.Unix(0, 0))
		runner := bench.NewBenchRunner(
			bench.WithStore[string](store), bench.WithClock[string](clock),
			bench.WithProvenanceProbe[string](benchtest.NewFixedProvenanceProbe(benchtest.WithGitCommit("fixture"), benchtest.WithHost("fixture"), benchtest.WithOS("linux"), benchtest.WithArch("amd64"))),
			bench.WithIDSuffix[string](func() string { return id }),
			bench.WithMetrics[string](metric.ExactMatch[string](), metric.BLEU(), metric.ROUGE1(), metric.ROUGE2(), metric.ROUGEL()),
			bench.WithRepeats[string](2), bench.WithWarmup[string](1), bench.WithPercentileThreshold[string](threshold),
		)
		runner.Register("model", &observedFixture{clock: clock}, bench.WithSubject(bench.SubjectIdentity{Name: subject, ModelDigest: subject}))
		result, err := runner.Run(ctx, dataset)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := store.Load(ctx, result.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded, result) {
			t.Fatalf("reload differs:\n%+v\n%+v", result, loaded)
		}
		return loaded
	}
	a, b := run("aaaaaaaa", "subject-a", 4096), run("bbbbbbbb", "subject-b", 1)
	p := a.Branches["model"].Performance
	if p.Observations != 4 || p.Failures != 0 || p.Latency.P50 != 10 || p.Latency.P95 != 10 || p.Latency.P99 != 10 || p.Latency.Mean != 10 || p.TTFT == nil || p.TTFT.P50 != 2 || p.OutputTokens != 8 || p.InputTokens != 12 || p.OutputTPS != 200 || p.TotalTPS != 500 {
		t.Fatalf("performance: %+v", p)
	}
	if !reflect.DeepEqual(p, b.Branches["model"].Performance) {
		t.Fatal("radix and in-memory percentiles differ")
	}
	for _, m := range a.Metrics[:5] {
		if math.Abs(m.Value-1) > 1e-9 {
			t.Fatalf("%s=%g", m.Name, m.Value)
		}
	}
	if a.EvaluationFingerprint != b.EvaluationFingerprint || a.Branches["model"].SubjectFingerprint == b.Branches["model"].SubjectFingerprint {
		t.Fatal("subject and evaluation identity conflated")
	}
	diff, err := bench.CompareSamples(ctx, store, store, a, b)
	if err != nil || diff.Verdict() != "passed" {
		t.Fatalf("comparison: %+v %v", diff, err)
	}
	for _, tc := range []struct {
		name     string
		change   func(*bench.RunResult)
		eligible bool
	}{
		{"identical", func(*bench.RunResult) {}, true},
		{"subject", func(r *bench.RunResult) {
			r.Branches["other"] = bench.BranchResult{Subject: bench.SubjectIdentity{Name: "other"}}
		}, true},
		{"metric", func(r *bench.RunResult) { r.Evaluation.Metrics[0].Version = "changed" }, false},
		{"metric-config", func(r *bench.RunResult) { r.Evaluation.Metrics[0].Config = map[string]string{"mode": "changed"} }, false},
		{"metric-missing", func(r *bench.RunResult) { r.Evaluation.Metrics = r.Evaluation.Metrics[1:] }, false},
		{"judge", func(r *bench.RunResult) {
			r.Evaluation.Judges = []bench.JudgeProvenance{{Metric: "judge", Model: "changed", PromptVersion: "1", PromptFingerprint: "changed"}}
		}, false},
		{"dataset", func(r *bench.RunResult) { r.Evaluation.Dataset = "changed" }, false},
		{"tokenizer", func(r *bench.RunResult) { r.Evaluation.Tokenizers = []string{"changed"} }, false},
		{"execution", func(r *bench.RunResult) { r.Evaluation.Execution.Concurrency++ }, false},
		{"missing", func(r *bench.RunResult) { r.Evaluation = bench.EvaluationIdentity{} }, false},
		{"unsupported", func(r *bench.RunResult) { r.Evaluation.Version = "99" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			var target bench.RunResult
			if err := json.Unmarshal(raw, &target); err != nil {
				t.Fatal(err)
			}
			tc.change(&target)
			diff := bench.NewRunComparator().Compare(a, &target)
			if diff.Eligibility.Eligible != tc.eligible {
				t.Fatalf("eligibility: %+v", diff.Eligibility)
			}
			for _, reporter := range []report.Reporter{report.JSON(), report.CSV(), report.Markdown(), report.Table(), report.HTML(), report.VegaLite(), report.JUnit(report.WithBounds(bench.Bound{Metric: "latency_ms", Kind: bench.Max, Limit: 10}))} {
				var output bytes.Buffer
				if err := reporter.Generate(ctx, &output, report.Input{Result: &target, Store: store, Diff: diff}); err != nil {
					t.Fatal(err)
				}
				text := strings.ToLower(output.String())
				if !strings.Contains(text, "eligibility") || !strings.Contains(text, diff.Verdict()) {
					t.Fatalf("%s omits comparison outcome", reporter.Name())
				}
			}
		})
	}
	t.Logf("fingerprint=%s dataset=%s latency=10ms TTFT=2ms output=200tps total=500tps records=4", a.EvaluationFingerprint, a.Provenance.DatasetHash)
}

type generatedIterator struct {
	count, index int
	input        []byte
}

func (it *generatedIterator) Next(ctx context.Context) (bench.Sample[string], bool, error) {
	if err := ctx.Err(); err != nil {
		return bench.Sample[string]{}, false, err
	}
	if it.index == it.count {
		return bench.Sample[string]{}, false, nil
	}
	s := bench.Sample[string]{ID: fmt.Sprint(it.index), Input: it.input, Label: "yes"}
	it.index++
	return s, true, nil
}
func (it *generatedIterator) Close() error { return nil }

func generatedDataset(count int) bench.Dataset[string] {
	return bench.NewGeneratorDataset(bench.DatasetDescriptor{Name: "generated", Version: "1"}, func(context.Context) (stream.Iterator[bench.Sample[string]], error) {
		return &generatedIterator{count: count, input: bytes.Repeat([]byte("x"), 256)}, nil
	})
}

type heapMetric struct {
	heap  *uint64
	count int
}

func (m *heapMetric) Identity() bench.MetricIdentity {
	return bench.MetricIdentity{Name: "heap", Version: "1"}
}

func (m *heapMetric) Compute([]bench.ScoredSample[string]) bench.MetricResult {
	return bench.MetricResult{Name: "heap"}
}
func (m *heapMetric) NewAccumulator() bench.Accumulator[string] { return m }
func (m *heapMetric) Add(bench.ScoredSample[string]) error {
	m.count--
	if m.count == 0 {
		runtime.GC()
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		*m.heap = memory.HeapAlloc
	}
	return nil
}
func (m *heapMetric) Result() bench.MetricResult { return bench.MetricResult{Name: "heap"} }

// Runs serially because live process heap measurements cannot be isolated across parallel tests.
func TestEvaluationMemoryStorageBudgets(t *testing.T) {
	var executionHeap, metricHeap, storedBytes [2]uint64
	for index, count := range []int{1000, 40000} {
		objects := bench.NewDirStore(t.TempDir())
		store := bench.NewResultStore(objects)
		clock := util.NewFakeClock(time.Unix(0, 0))
		calls := 0
		runner := bench.NewBenchRunner(
			bench.WithStore[string](store), bench.WithClock[string](clock),
			bench.WithMetrics[string](&heapMetric{heap: &metricHeap[index], count: count}, metric.ExactMatch[string]()),
		)
		runner.Register("model", bench.EvaluatorFunc("model", func(context.Context, []byte) (bench.Prediction[string], error) {
			calls++
			clock.Advance(time.Duration(calls%100+1) * time.Microsecond)
			if calls == count {
				runtime.GC()
				var memory runtime.MemStats
				runtime.ReadMemStats(&memory)
				executionHeap[index] = memory.HeapAlloc
			}
			return bench.Prediction[string]{Label: "yes"}, nil
		}))
		result, err := runner.Run(context.Background(), generatedDataset(count))
		if err != nil {
			t.Fatal(err)
		}
		p := result.Branches["model"].Performance.Latency
		if p.P50 != .05 || p.P95 != .095 || p.P99 != .099 {
			t.Fatalf("exact percentiles: %+v", p)
		}
		keys, err := objects.List(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range keys {
			data, err := objects.Get(context.Background(), key, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			storedBytes[index] += uint64(len(data))
		}
		if storedBytes[index] > 256<<20 {
			t.Fatalf("quota exceeded: %d", storedBytes[index])
		}
		var output bytes.Buffer
		if err := report.Markdown().Generate(context.Background(), &output, report.Input{Result: result, Store: store}); err != nil {
			t.Fatal(err)
		}
		if output.Len() > 32768 {
			t.Fatalf("report exceeds 32 KiB: %d", output.Len())
		}
	}
	for _, heaps := range [][2]uint64{executionHeap, metricHeap} {
		if heaps[1] > heaps[0]+4<<20 {
			t.Fatalf("heap growth exceeds 4 MiB: %v", heaps)
		}
	}
	if storedBytes[1] > storedBytes[0]*45 {
		t.Fatalf("storage growth exceeds linear bound: %v", storedBytes)
	}
	t.Logf("inputs=1000,40000 bytes/input=256 execution_heap=%v metric_heap=%v stored_bytes=%v", executionHeap, metricHeap, storedBytes)
}

func TestEvaluationQuotaCleanup(t *testing.T) {
	t.Parallel()
	objects := bench.NewDirStore(t.TempDir())
	limits := bench.DefaultLimits()
	limits.MaxRunBytes = 200
	limits.SegmentBytes = 100
	runner := bench.NewBenchRunner(bench.WithStore[string](bench.NewResultStore(objects)), bench.WithLimits[string](limits))
	runner.Register("model", bench.EvaluatorFunc("model", func(context.Context, []byte) (bench.Prediction[string], error) {
		return bench.Prediction[string]{}, nil
	}))
	result, err := runner.Run(context.Background(), generatedDataset(100))
	var runErr *bench.RunError
	if result != nil || !errors.As(err, &runErr) || runErr.Outcome != bench.OutcomeLimitExceeded {
		t.Fatalf("result=%v error=%v", result, err)
	}
	keys, err := objects.List(context.Background(), "")
	if err != nil || len(keys) != 0 {
		t.Fatalf("owned objects remain: %v %v", keys, err)
	}
}

func TestEvaluationFailedZeroPrediction(t *testing.T) {
	t.Parallel()
	store := bench.NewResultStore(bench.NewDirStore(t.TempDir()))
	runner := bench.NewBenchRunner(bench.WithStore[string](store), bench.WithMetrics[string](metric.ExactMatch[string]()))
	runner.Register("error", bench.EvaluatorFunc("error", func(context.Context, []byte) (bench.Prediction[string], error) { panic("fixture panic") }))
	result, err := runner.Run(context.Background(), bench.NewSliceDataset(bench.DatasetDescriptor{Name: "failure", Version: "1"}, []bench.Sample[string]{{ID: "zero", Label: ""}}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value != 0 || result.Branches["error"].Performance.Failures != 1 {
		t.Fatal("failed zero prediction counted as correct")
	}
	if slices.ContainsFunc(result.Metrics, func(m bench.MetricResult) bool { return m.Name == "ttft_ms" }) {
		t.Fatal("invented TTFT")
	}
}
