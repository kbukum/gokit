# bench

**General-purpose accuracy and quality benchmarking framework for Go**

> **Note**:
> This package is for **model/system quality evaluation** (accuracy, ranking, calibration, regression),
> not Go micro-benchmarks. For CPU/memory micro-benchmarks see `go test -bench`
> and the per-package `*_test.go` files.

Think of `bench` as `testing.B` for classification accuracy, ranking quality,
probability calibration, and regression error. Evaluators are backed by gokit **providers**,
datasets flow through **pipelines**, and metrics are fully pluggable.

## Features

- **Generics-first** — `Sample[L]`, `Prediction[L]`, `Evaluator[L]` are parameterised on the label type
- **Provider integration** — any `provider.RequestResponse` becomes an evaluator with one adapter call
- **Pipeline integration** — datasets expose a `stream.Pipeline` / `stream.Iterator` for lazy, backpressure-aware loading
- **Pluggable metrics** — classification, probability, ranking, regression, matching —
  or bring your own
- **Multiple output formats** — JSON, Markdown, CSV, HTML, JUnit XML, Vega-Lite, SVG visualisations
- **Comparison & regression detection** — diff two runs, surface fixed/regressed samples,
  gate CI on thresholds
- **CLI helpers** — `CLIRunner` wires up run → store → compare → print in a few lines
- **Concurrent evaluation** — fan out across evaluators with configurable concurrency and per-sample timeouts
- **Reproducible runs** — every `RunResult` carries a `RunProvenance` record (seed, git commit, tool/host/os/arch, and an order-independent dataset hash); inject a `ProvenanceProbe` and a run seed for deterministic, auditable runs

## Install

```bash
go get github.com/kbukum/gokit/bench@latest
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/bench/metric"
	"github.com/kbukum/gokit/bench/report"
)

func main() {
	ctx := context.Background()

	// 1. Define an evaluator (wraps any prediction function).
	eval := bench.EvaluatorFunc("my-classifier",
		func(ctx context.Context, input []byte) (bench.Prediction[string], error) {
			// Replace with your model / API call.
			return bench.Prediction[string]{
				Label: "positive",
				Score: 0.92,
				Scores: map[string]float64{
					"positive": 0.92,
					"negative": 0.08,
				},
			}, nil
		},
	)

	// 2. Create a runner with metrics.
	clf, err := metric.BinaryClassification[string]("positive")
	if err != nil {
		return err
	}
	runner := bench.NewBenchRunner(
		bench.WithTag[string]("v1.0"),
		bench.WithConcurrency[string](8),
		bench.WithMetrics(
			metric.AsRunMetric(clf),
			metric.AsRunMetric(metric.AUCROC[string]("positive")),
			metric.AsRunMetric(metric.BrierScore[string]("positive")),
		),
	)

	// 3. Register one or more evaluators (branches).
	runner.Register("baseline", eval)

	// 4. Load a dataset (directory with manifest.json + sample files).
	dataset := bench.NewDatasetLoader("./testdata", func(s string) (string, error) {
		return s, nil // string labels → string
	})

	// 5. Run the benchmark.
	result, err := runner.Run(ctx, dataset)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 6. Generate a Markdown report.
	_ = report.Markdown().Generate(os.Stdout, result)
}
```

## Key Types & Functions

| Symbol | Kind | Description |
|--------|------|-------------|
| `Sample[L]` | struct | Labeled data point — ID, Input, Label, Source, Metadata |
| `Prediction[L]` | struct | Evaluator output — Label, Score, per-label Scores, Metadata |
| `ScoredSample[L]` | struct | Pairs a `Sample` with its `Prediction` |
| `Evaluator[L]` | interface | `provider.RequestResponse[[]byte, Prediction[L]]` |
| `EvaluatorFunc[L]` | func | Wraps a plain `func(ctx, []byte) (Prediction[L], error)` as an `Evaluator` |
| `FromProvider[I,O,L]` | func | Adapts any `provider.RequestResponse[I,O]` into an `Evaluator[L]` |
| `DatasetLoader[L]` | struct | Reads a manifest directory into `[]Sample[L]` or a `stream.Pipeline` |
| `LabelMapper[L]` | func | `func(string) (L, error)` — converts manifest string labels to typed `L` |
| `BenchRunner[L]` | struct | Orchestrates evaluation: load → evaluate → compute metrics → store |
| `RunResult` | struct | Full benchmark output — metrics, branch results, per-sample details, curves, provenance |
| `RunProvenance` | struct | Reproducibility record on every `RunResult` — seed, RNG algorithm, git commit, tool/host/os/arch, order-independent dataset hash, branch and metric names, and (when judge metrics scored the run) a `Judges` list recording each judge's model, resolved backend model, prompt version, and rubric fingerprint |
| `ProvenanceProbe` | interface | Injected source of host and git-commit provenance — `GitCommit`/`Host`/`OS`/`Arch` |
| `SystemProvenanceProbe` | struct | Default probe — host/os/arch from the runtime, git commit best-effort from CI env vars |
| `RunComparator` | struct | Diffs two `RunResult`s, reports metric changes & sample regressions |
| `CLIRunner` | struct | Convenience wrapper: run, compare, list, show — writes to `io.Writer` |
| `FileStorage` | struct | Stores `RunResult` as JSON files on disk |
| `RunStorage` | interface | Save / Load / Latest / List for benchmark results |

## Sub-packages

| Package | Description |
|---------|-------------|
| [`bench/metric`](metric/) | Metric implementations — classification, probability, ranking, regression, matching, token usage, semantic similarity, and LLM judge (context metrics) |
| [`bench/report`](report/) | Output-format reporters — JSON, Markdown, CSV, Table, JUnit, Vega-Lite, HTML |
| [`bench/viz`](viz/) | Pure-Go SVG visualisation generation — ROC, confusion matrix, calibration, distribution, comparison |
| [`bench/storage`](storage/) | Cloud-storage adapter for bench results — wraps `gokit/storage` |
| [`bench/testutil`](testutil/) | Deterministic test doubles — `FixedProvenanceProbe` for offline, reproducible provenance |

## Reproducibility

Every run records a `RunProvenance` on its `RunResult`: the deterministic seed and RNG algorithm, the source-control commit, the tool and host identity, and an order-independent content hash of the evaluated dataset (folded from each sample's id, input, and label). Provenance is gathered through an injected `ProvenanceProbe`, so runs are reproducible and auditable, and unit tests stay offline and deterministic.

```go
runner := bench.NewBenchRunner[string](
    bench.WithSeed[string](42),
    bench.WithClock[string](clock),           // deterministic timestamps & IDs
    bench.WithProvenanceProbe[string](probe), // host/os/arch + git commit
)
```

The default `SystemProvenanceProbe` reads host/os/arch from the runtime and resolves the git commit best-effort from CI environment variables (`GITHUB_SHA` → `GIT_COMMIT` → `CI_COMMIT_SHA` → `SOURCE_COMMIT`), taking no dependency on a git library. Tests inject `testutil.FixedProvenanceProbe` for fixed, offline values — with a fixed clock, fixed probe, and seed, a run's JSON is byte-identical across executions.

## Available Metrics

### Classification

| Constructor | Description |
|-------------|-------------|
| `BinaryClassification[L](positiveLabel, ...ClassificationOption)` | Precision, recall, F1, accuracy, FPR + confusion counts. Folds the decision threshold into the metric name (`classification[t0.5]`) and returns an error for a non-finite or out-of-`[0, 1]` threshold |
| `ConfusionMatrix[L](labels)` | Full N×N confusion matrix |
| `ThresholdSweep[L](positiveLabel, thresholds)` | Metrics at each threshold (default 0.1–0.9) |
| `MultiClassClassification[L](labels)` | Macro / micro / weighted precision, recall, F1 |

### Probability & Calibration

| Constructor | Description |
|-------------|-------------|
| `AUCROC[L](positiveLabel)` | Area under the ROC curve |
| `BrierScore[L](positiveLabel)` | Mean squared error of predicted probabilities (lower is better) |
| `LogLoss[L](positiveLabel)` | Logarithmic loss (cross-entropy) |
| `Calibration[L](positiveLabel, bins)` | Calibration curve — predicted probability vs actual frequency |

### Ranking

| Constructor | Description |
|-------------|-------------|
| `NDCG[L](k)` | Normalised Discounted Cumulative Gain at *k* |
| `MAP[L](positiveLabel)` | Mean Average Precision |
| `PrecisionAtK[L](positiveLabel, k)` | Precision at top *k* |
| `RecallAtK[L](positiveLabel, k)` | Recall at top *k* |

### Regression

| Constructor | Description |
|-------------|-------------|
| `MAE()` | Mean Absolute Error (`Metric[float64]`) |
| `MSE()` | Mean Squared Error |
| `RMSE()` | Root Mean Squared Error |
| `RSquared()` | Coefficient of determination (R²) |

### Matching

| Constructor | Description |
|-------------|-------------|
| `ExactMatch[L]()` | Fraction of exact label matches |
| `FuzzyMatch(threshold)` | Levenshtein-based string similarity (`Metric[string]`). Folds the threshold into the metric name (`fuzzy_match[t0.8]`) and returns an error for a non-finite or out-of-`[0, 1]` threshold |

### Composite

| Constructor | Description |
|-------------|-------------|
| `Weighted[L](weights)` | Weighted combination of multiple metrics |

Use `metric.AsRunMetric` / `metric.AsRunMetrics` to pass any `Metric[L]` into `bench.WithMetrics`.

### Token usage

| Constructor | Description |
|-------------|-------------|
| `TokenStats[L](counter)` | Total / average predicted and reference token count via an injected `llm.TokenCounter`; descriptive, so run comparison excludes it from regression classification |

### Semantic (context metric)

`SemanticSimilarity` is a **context metric**: it performs I/O (embedding text), so it takes a `context.Context` and may fail, rather than the pure `Metric` contract. Register context metrics with `bench.WithContextMetrics` (adapted via `metric.AsRunContextMetric`); the runner computes pure metrics first, then context metrics, each bounded by a timeout and honoring cancellation.

Each batch is embedded in its own provider call routed through the canonical `resilience.Policy` (default: a per-call 30s timeout, no retries — embedding is idempotent, so callers may add bounded retries), so a large run is not scored against a single dataset-wide deadline, and each response is validated to be a well-formed index permutation of its batch before use. A provider timeout or cancellation surfaces as a typed timeout/canceled `AppError`; a malformed or non-finite response as an external-service error. The metric name embeds a stable, escaped model identity built from provider, name, and version, together with the configured threshold — for example `semantic_similarity[openai/text-embedding-3-small@v1:t0.8]` — so runs scored by different models, versions, or thresholds never join as compatible by name alone (`match_rate` is a fraction at a fixed cutoff, so comparing it across thresholds would be unsound); the provider name is used only when the model carries no identity metadata.

| Constructor | Description |
|-------------|-------------|
| `SemanticSimilarity[L](provider, model, opts...)` | Mean embedding cosine similarity of prediction vs reference via an injected `embedding.Provider` and `ai/vector.CosineSimilarity`, plus a threshold match rate |

Options: `WithSemanticThreshold` (must be a finite value in `[-1, 1]`), `WithSemanticTimeout` (per-call timeout on the policy), `WithSemanticPolicy` (full `resilience.Policy`), `WithSemanticBatchSize`, `WithSemanticExtractor`. A resolved context-metric `Result` can be surfaced as a pure `Metric` with `metric.AsSync` (the precompute path).

```go
semantic, err := metric.SemanticSimilarity[string](provider, model)
if err != nil {
	return err
}
clf, err := metric.BinaryClassification[string]("positive")
if err != nil {
	return err
}
runner := bench.NewBenchRunner(
	bench.WithMetrics(metric.AsRunMetric(clf)),
	bench.WithContextMetrics(metric.AsRunContextMetric(semantic)),
)
```

### LLM judge (context metric)

`LLMJudge` is a **context metric** that grades each prediction against its reference by asking an injected `llm.Provider` to score the pair, using a **versioned** `JudgePrompt` so a run records exactly which prompt produced its scores. It reports the mean judge score as the primary value and a threshold pass rate; the judge model and prompt identity are recorded in the result detail and lifted into `RunProvenance`.

Model output is treated as **untrusted**: the judge is asked for a JSON object, and the reply is parsed into a typed `JudgeVerdict` with shape and range validation. A malformed or non-JSON reply, an out-of-range or missing score, a completion that did not finish normally (truncated by the token cap, stopped by a content filter, ended by a provider error), or an over-long reply surfaces as a typed external-service `AppError` (the untrusted model returned an unusable response) — never a fabricated success-shaped score and never a panic. This parser is the trust boundary on reply shape, not a prompt-injection detector; the injection defense is the data-only framing in the prompt's system instruction. Every provider call is bounded by the canonical `resilience.Policy` (default: a per-call 30s timeout, and an injected policy must itself carry a positive timeout) and calls fan out with bounded concurrency (default 4), never an unbounded goroutine fan-out; the first sample failure cancels a derived context and stops scheduling the rest, so one bad reply does not bill every remaining call. A `JudgePrompt` version must be a strict semver, so a run's judge provenance is always a comparable identity. When a provider resolves the request to a single backend model that differs from the one requested, that resolved id is recorded in provenance; a run whose samples resolve to more than one model is rejected as incomparable. The metric name embeds the model and prompt identity, including a fingerprint of the rubric (template body + system instruction) — for example `llm_judge[openai/gpt-4o-mini@gokit.builtin.judge@1.0.0#a1b2c3d4e5f6:t0.5]` — so runs scored by a different judge model, prompt, rubric, or threshold never join as compatible by name alone; editing the rubric without bumping the version still changes the fingerprint and thus the identity.

| Constructor | Description |
|-------------|-------------|
| `LLMJudge[L](provider, model, prompt, opts...)` | Mean LLM-judge score of prediction vs reference via an injected `llm.Provider` and a versioned `JudgePrompt`, plus a threshold pass rate |
| `ParseJudgePrompt(id, version, template)` | Parses a custom judge prompt, requiring exactly `{reference}` and `{prediction}` placeholders |
| `DefaultJudgePrompt()` | The built-in judge rubric with a defensive JSON-only system instruction |

Options: `WithJudgeThreshold` (finite value in `[0, 1]`), `WithJudgeTimeout` (per-call timeout on the policy), `WithJudgePolicy` (full `resilience.Policy`), `WithJudgeConcurrency` (bounded fan-out, must be positive), `WithJudgeMaxTokens`, `WithJudgeExtractor`.

```go
judge, err := metric.LLMJudge[string](provider, "gpt-4o-mini", metric.DefaultJudgePrompt())
if err != nil {
	return err
}
runner := bench.NewBenchRunner(
	bench.WithContextMetrics(metric.AsRunContextMetric(judge)),
)
```

## Reporters

| Constructor | Output |
|-------------|--------|
| `report.JSON()` | Canonical Bench JSON with `$schema` and version |
| `report.HTML()` | Self-contained HTML with embedded Vega-Lite charts |
| `report.Markdown()` | GitHub-flavoured Markdown tables |
| `report.CSV()` | Flat CSV — one row per metric |
| `report.JUnit(opts...)` | JUnit XML — metrics become test cases, gated by targets |
| `report.VegaLite()` | Vega-Lite spec JSON (`{ filename: spec, … }`) |

### SVG Visualisations (`bench/viz`)

| Function | Description |
|----------|-------------|
| `viz.RenderAll(result, ...RenderOption)` | All available SVGs as `map[string]string` |
| `viz.RenderROC(roc)` | ROC curve |
| `viz.RenderConfusion(cm)` | Confusion-matrix heatmap |
| `viz.RenderCalibration(cal)` | Calibration curve |
| `viz.RenderDistribution(dists)` | Score-distribution histograms |
| `viz.RenderComparison(branches)` | Branch comparison grouped bar chart |

## Usage Examples

### Multi-class Classification

```go
labels := []string{"cat", "dog", "bird"}

runner := bench.NewBenchRunner(
	bench.WithMetrics(
		metric.AsRunMetric(metric.MultiClassClassification(labels)),
		metric.AsRunMetric(metric.ConfusionMatrix(labels)),
	),
)
```

### Regression

```go
runner := bench.NewBenchRunner(
	bench.WithMetrics(
		metric.AsRunMetric(metric.RMSE()),
		metric.AsRunMetric(metric.RSquared()),
	),
)
```

### Adapting an Existing Provider

```go
eval := bench.FromProvider(
	myProvider,                              // provider.RequestResponse[MyInput, MyOutput]
	func(raw []byte) MyInput { ... },        // []byte → provider input
	func(out MyOutput) bench.Prediction[string] { ... }, // provider output → Prediction
)
runner.Register("my-provider", eval)
```

### CI / CD with JUnit Targets

```go
targets := map[string]float64{"f1": 0.90, "accuracy": 0.85}

clf, err := metric.BinaryClassification[string]("positive")
if err != nil {
	return err
}
runner := bench.NewBenchRunner(
	bench.WithTargets[string](targets),
	bench.WithFailOnRegression[string](true),
	bench.WithMetrics(
		metric.AsRunMetric(clf),
	),
)

// JUnit reporter uses the same targets to pass/fail test cases.
junit := report.JUnit(report.WithTargets(targets))
_ = junit.Generate(junitFile, result)
```

## Comparison & Regression Detection

```go
cmp := bench.NewRunComparator(bench.WithChangeThreshold(0.02))

diff := cmp.Compare(baseResult, latestResult)

fmt.Println(diff.Summary())
// e.g. "f1: 0.91 → 0.93 (+0.02 ✓) | accuracy: 0.88 → 0.86 (−0.02 ✗)"

if diff.HasRegression() {
	fmt.Printf("Regressed samples: %v\n", diff.Regressed)
	os.Exit(1)
}
```

## CLI Helper

```go
store := bench.NewFileStorage("./results")
cli := bench.NewCLIRunner(store, bench.WithOutput(os.Stdout))

_ = cli.RunAndPrint(ctx, runner, dataset)  // run + print report
_ = cli.CompareLatest(ctx)                 // diff last two runs
_ = cli.ListRuns(ctx)                      // list stored runs
_ = cli.ShowRun(ctx, "run-abc123")         // show a specific run
```

## Related Packages

- [**provider**](../provider/) — `Evaluator` is a `provider.RequestResponse` under the hood
- [**stream**](../stream/) — `DatasetLoader.Pipeline()` returns a lazy `stream.Pipeline`
- [**process**](../process/) — wrap a subprocess as a provider, then adapt to an evaluator
- [**storage**](../storage/) — `bench/storage` adapts `gokit/storage` for cloud result persistence

## License

[MIT](../LICENSE) — Copyright (c) 2024 kbukum

[← Back to main gokit README](../README.md)
