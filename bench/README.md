# bench

**General-purpose accuracy and quality benchmarking framework for Go**

Think of `bench` as `testing.B` for classification accuracy, ranking quality, probability
calibration, and regression error. Evaluators are backed by gokit **providers**, datasets
flow through **pipelines**, and metrics are fully pluggable.

## Features

- **Generics-first** — `Sample[L]`, `Prediction[L]`, `Evaluator[L]` are parameterised on the label type
- **Provider integration** — any `provider.RequestResponse` becomes an evaluator with one adapter call
- **Pipeline integration** — datasets expose a `pipeline.Pipeline` / `pipeline.Iterator` for lazy, backpressure-aware loading
- **Pluggable metrics** — classification, probability, ranking, regression, matching — or bring your own
- **Multiple output formats** — JSON, Markdown, CSV, HTML, JUnit XML, Vega-Lite, SVG visualisations
- **Comparison & regression detection** — diff two runs, surface fixed/regressed samples, gate CI on thresholds
- **CLI helpers** — `CLIRunner` wires up run → store → compare → print in a few lines
- **Concurrent evaluation** — fan out across evaluators with configurable concurrency and per-sample timeouts

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
	runner := bench.NewBenchRunner(
		bench.WithTag[string]("v1.0"),
		bench.WithConcurrency[string](8),
		bench.WithMetrics(
			metric.AsRunMetric(metric.BinaryClassification[string]("positive")),
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
| `DatasetLoader[L]` | struct | Reads a manifest directory into `[]Sample[L]` or a `pipeline.Pipeline` |
| `LabelMapper[L]` | func | `func(string) (L, error)` — converts manifest string labels to typed `L` |
| `BenchRunner[L]` | struct | Orchestrates evaluation: load → evaluate → compute metrics → store |
| `RunResult` | struct | Full benchmark output — metrics, branch results, per-sample details, curves |
| `RunComparator` | struct | Diffs two `RunResult`s, reports metric changes & sample regressions |
| `CLIRunner` | struct | Convenience wrapper: run, compare, list, show — writes to `io.Writer` |
| `FileStorage` | struct | Stores `RunResult` as JSON files on disk |
| `RunStorage` | interface | Save / Load / Latest / List for benchmark results |

## Sub-packages

| Package | Description |
|---------|-------------|
| [`bench/metric`](metric/) | Metric implementations — classification, probability, ranking, regression, matching |
| [`bench/report`](report/) | Output-format reporters — JSON, Markdown, CSV, Table, JUnit, Vega-Lite, HTML |
| [`bench/viz`](viz/) | Pure-Go SVG visualisation generation — ROC, confusion matrix, calibration, distribution, comparison |
| [`bench/storage`](storage/) | Cloud-storage adapter for bench results — wraps `gokit/storage` |

## Available Metrics

### Classification

| Constructor | Description |
|-------------|-------------|
| `BinaryClassification[L](positiveLabel, ...ClassificationOption)` | Precision, recall, F1, accuracy, FPR + confusion counts |
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
| `FuzzyMatch(threshold)` | Levenshtein-based string similarity (`Metric[string]`) |

### Composite

| Constructor | Description |
|-------------|-------------|
| `Weighted[L](weights)` | Weighted combination of multiple metrics |

Use `metric.AsRunMetric` / `metric.AsRunMetrics` to pass any `Metric[L]` into `bench.WithMetrics`.

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

runner := bench.NewBenchRunner(
	bench.WithTargets[string](targets),
	bench.WithFailOnRegression[string](true),
	bench.WithMetrics(
		metric.AsRunMetric(metric.BinaryClassification[string]("positive")),
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
- [**pipeline**](../pipeline/) — `DatasetLoader.Pipeline()` returns a lazy `pipeline.Pipeline`
- [**process**](../process/) — wrap a subprocess as a provider, then adapt to an evaluator
- [**storage**](../storage/) — `bench/storage` adapts `gokit/storage` for cloud result persistence

## License

[MIT](../LICENSE) — Copyright (c) 2024 kbukum

[← Back to main gokit README](../README.md)
