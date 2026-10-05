# bench

Typed quality and performance evaluation for Go providers. This is not a replacement for Go microbenchmarks.

## Run, reload, report, compare

```go
func evaluate(ctx context.Context, output io.Writer, directory string) error {
	store := bench.NewResultStore(bench.NewDirStore(directory))
	dataset := bench.NewSliceDataset(
		bench.DatasetDescriptor{Name: "example", Version: "1"},
		[]bench.Sample[string]{{ID: "one", Input: []byte("hello"), Label: "hello"}},
	)
	runner := bench.NewBenchRunner(
		bench.WithStore[string](store),
		bench.WithMetrics[string](metric.ExactMatch[string](), metric.BLEU()),
		bench.WithRepeats[string](2),
		bench.WithWarmup[string](1),
	)
	runner.Register("echo", bench.EvaluatorFunc("echo",
		func(ctx context.Context, input []byte) (bench.Prediction[string], error) {
			return bench.Prediction[string]{Label: string(input)}, ctx.Err()
		}),
		bench.WithSubject(bench.SubjectIdentity{Name: "echo", ModelDigest: "example-v1"}),
	)
	result, err := runner.Run(ctx, dataset)
	if err != nil {
		return err
	}
	reloaded, err := store.Load(ctx, result.ID)
	if err != nil {
		return err
	}
	diff := bench.NewRunComparator().Compare(result, reloaded)
	return report.Markdown().Generate(ctx, output, report.Input{
		Result: reloaded, Store: store, Diff: diff,
	})
}
```

Imports: `context`, `io`, `github.com/kbukum/gokit/bench`, `github.com/kbukum/gokit/bench/metric`, and `github.com/kbukum/gokit/bench/report`.

`TestConsumerEvaluationPath` is the executable consumer proof. It checks real files, reloads, exact metrics, all seven reporters, subject changes, incompatible evaluation identity, and missing identity. No model download or network is needed.

## Execution and observations

`Dataset[L]` exposes `Describe(ctx)` and `Iterator(ctx)`. `NewSliceDataset` uses caller-owned samples; `NewGeneratorDataset` opens fresh iterators. `DatasetLoader` streams `manifest.json` through `json.Decoder`, loads each referenced input lazily, and confines file paths. Its `Pipeline` method remains available for composition. There is no whole-dataset `All` or `Manifest` API.

One coordinator admits at most `C` evaluators and keeps at most `2C` uncommitted observations. Records commit in sample-major, repeat order. Each branch opens a fresh dataset traversal; differing content fails the run. Evaluators must honor context cancellation. The runner stops admission on cancellation, waits for admitted evaluations, and recovers evaluator panics as failed observations.

`ObservedEvaluator[L]` optionally implements `ExecuteObserved(ctx, input, observer)`. The first `ContentStarted` call records TTFT using the runner clock. `ReportUsage` replaces prior usage; `TokenUsage.Source` distinguishes `unavailable`, `reported`, and `counted`, including known zero. Counted usage includes the tokenizer identity.

`bench/llmeval.New` adapts `llm.Provider` with request and prediction mappers. Only nonempty text deltas start TTFT. Reasoning, tools, and metadata do not. Usage-only terminal events are retained. A configured `llm.TokenCounter` fills usage only when the runtime reports none. Unary evaluation has no invented TTFT.

Warmup runs the first `n` samples once per branch, without scoring. Its failures are counted separately. Each repeat is a scored observation. A sample is correct in sample comparisons only when every repeat succeeded and matched. Failed observations use zero predictions in quality metrics, count toward `failure_rate`, and are excluded from latency, TTFT, and throughput.

## Metrics and exact timing

Metrics expose `Identity() bench.MetricIdentity{Name, Version, Config}`. Names remain canonical and opaque, including `exact_match`, `ndcg@10`, and `fuzzy_match[t0.8]`. Algorithm version and configuration are independent identity fields. Configuration is not a measured score.

`metric.Result` aliases `bench.MetricResult`. Judge provenance, confusion matrices, ROC, calibration, threshold sweeps, and weighted components are typed fields. No untyped detail payload is used for measurement or identity.

| Metric | Meaning |
|---|---|
| `latency_ms` | P50 headline; P50/P95/P99/mean values, lower is better |
| `ttft_ms` | Same exact summaries for observed content-only TTFT; omitted if unavailable |
| `throughput_output_tps` | Successful known-usage output tokens divided by their summed latency seconds |
| `throughput_total_tps` | Successful known-usage input plus output tokens over the same denominator |
| `failure_rate` | Failed observations divided by all scored observations |
| `usage_tokens` | Neutral input/output totals and the number of successful observations without usage |

Latency runs from immediately before evaluator execution until return. Records store nanoseconds. Percentiles use nearest rank, `ceil(p*n/100)`, over successful observations. Up to 4,096 observations sort in memory; larger sets use eight radix passes over stored records, solving all latency and TTFT ranks together. `WithPercentileThreshold` can lower the cutoff for tests.

Exact/fuzzy matching, BLEU, ROUGE, MAE, MSE, and RMSE stream through independent accumulators. Classification, probability/calibration, ranking, R-squared, weighted, semantic, and judge metrics remain available. Whole-set metrics collect only up to `MaxRetainedSamples`; larger runs fail explicitly instead of approximating.

BLEU matches sacreBLEU **2.4.3** corpus BLEU: 13a tokenizer, exponential smoothing, max order four, case-sensitive, no effective order, scaled to 0-1. ROUGE-1/2/L match **rouge-score 0.1.2**, default tokenizer without stemming, averaging per-sample F with mean P/R. Python **3.11.6** generated the pinned reference cases; absolute tolerance is **1e-9**. Empty and one-word BLEU are zero without effective order.

## Storage and limits

`WithStore` is required. `ResultStore` owns quotas, cleanup, listing, loading, and the summary commit marker. Use `DirStore` for real local files, `MemoryStore` for small tests, or `bench/storage.NewProviderStorage` to adapt a kit storage backend.

Records live at `records/<id>/<branchIdx>/<segment>.jsonl`. The summary at `runs/<id>.json` is written **last**, after record flushes, metrics, fingerprints, and validation. A failed run returns no result and deletes its owned objects. Quota exhaustion is `OutcomeLimitExceeded`, not partial success. Corrupt summaries are errors, not silently skipped list entries.

`ResultStore.Records` returns a non-generic iterator of `Record` with `json.RawMessage` labels and display helpers. `Observations[L]` decodes typed labels lazily. Labels must round-trip through JSON to an equal value.

| Default limit | Value |
|---|---|
| Samples / branches / repeats / warmup | 100,000 / 16 / 100 / 100 |
| Maximum concurrency / reorder window | 64 / twice configured concurrency |
| Sample timeout / run budget / cleanup timeout | 5 minutes / 2 hours / 30 seconds |
| Encoded record / input | 1 MiB |
| Retained whole-set observations | 100,000 |
| Distribution labels | 1,024; larger distributions are omitted and flagged |
| Segment / per-run quota / summary read | 256 KiB / 256 MiB / 16 MiB |
| Sample-diff listed IDs | 100 per change, with exact counts |
| Markdown and HTML / table previews | 50 / 20 rows |

Configure production limits with `WithLimits`. Raising summary or segment sizes also requires `WithSummaryReadLimit` or `WithRecordReadLimit` on the store; readers never trust a stored document to increase their memory ceiling. Cleanup has an independent bounded context. Memory tests compare 1,000 with 40,000 generated samples of 256 input bytes and require less than 4 MiB live-heap growth during execution and metric processing.

## Identity, comparison, and reports

`RunProvenance` retains seed, named RNG, host, OS, architecture, build version, commit, judges, and an order-dependent framed dataset hash. `GitTreeState` is `clean` or `dirty` from build metadata; it is unknown when the commit comes from the environment. Inject `WithClock`, `WithIDSuffix`, and `WithProvenanceProbe` for deterministic results.

`SubjectIdentity` records what was tested. A subject change alone stays comparable. `EvaluationIdentity` version `"1"` pins the dataset, metric versions/configs, judges, observed tokenizers, and execution settings, including concurrency. Fingerprints are SHA-256 over canonical JSON.

`RunComparator.Compare` separates `Eligibility{Eligible, Complete, Reasons}` from measured regression. `Verdict()` is `ineligible`, `regressed`, or `passed`. Missing or unsupported identity is never a verified pass. `CompareSamples` reads ordered records and adds exact counts with bounded ID previews.

Every reporter uses `Generate(ctx, writer, report.Input{Result, Store, Diff})`. Store and Diff are optional. JSON, Markdown, table, CSV, HTML, Vega-Lite, and JUnit render eligibility and verdict when a diff is present. JUnit uses explicit `bench.Bound{Metric, Value, Kind: bench.Min|bench.Max, Limit}` through `report.WithBounds`. Missing metrics or subvalues fail closed, and comparison adds an eligibility test case.

The JSON schema is **2.0**. Detailed samples are not part of `RunResult`. The rskit mirror is pending; version 1.0 interchange is not claimed.

## Reproduce

From the gokit repository:

```sh
make test M=bench
make lint M=bench
make test M=bench/storage
make lint M=bench/storage
make auth-host-build
GOKIT_AUTH_HOST_BINARY="$PWD/auth/testhost/target/auth-host" \
  toven run test --module go:bench --module go:auth-testhost -- \
  -race -count=1 -timeout=20m -tags=integration
make check
```

[MIT license](../LICENSE) · [Module index](../README.md)
