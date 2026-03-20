# Bench — General-Purpose Benchmarking Module

## Overview

The `bench` package provides a general-purpose accuracy and quality benchmarking framework
for evaluating any system that produces predictions against labeled ground truth.

Think of it as `testing.B` for Go's microbenchmarks — but for **classification accuracy,
ranking quality, probability calibration, regression error**, and any other evaluation task
where you compare predicted outputs to known-correct labels.

The module is designed around gokit's existing patterns. An Evaluator **is** a
`provider.RequestResponse`. A Dataset **is** a `pipeline.Iterator`. Results persist via
`storage.Store`. Runs are observable via OpenTelemetry. This means every gokit capability —
middleware, resilience, concurrency, lifecycle management — works with bench out of the box.

## Motivation

The pykit and ruskit bench modules are tightly coupled to Sentinel's AI detection use case
(binary classification with `positive_label="ai_generated"`). We need a reusable framework
that any project can adopt for any evaluation task — sentiment analysis, document
categorization, spam detection, score prediction, ranking systems, or anything else that
produces measurable predictions.

The existing bench implementations have proven the architecture:
- **8 Python files, ~840 lines** (pykit/bench) — metrics, dataset, runner, storage, compare, report
- **5 Rust files, ~440 lines** (ruskit/bench) — metrics, dataset, storage, report (partial)

Both are ~80% generic. gokit/bench extracts and generalizes the 80% into a first-class module,
then adds multi-class, probability, ranking, regression, and matching metrics that pykit/bench
doesn't cover.

## Design Principles

1. **Evaluator = Provider** — Any `provider.RequestResponse[[]byte, Prediction[L]]` is an
   evaluator. Provider middleware (logging, metrics, tracing, retry, circuit breaker) applies
   automatically. No new abstraction needed.

2. **Dataset = Iterator** — `DatasetLoader` returns `pipeline.Iterator[Sample[L]]`, so datasets
   plug directly into pipeline operators (`Map`, `Filter`, `Parallel`, `Batch`).

3. **Metrics are pluggable** — The `Metric[L]` interface takes scored samples and returns
   results. Built-in metrics cover the common categories; users add domain-specific metrics
   by implementing one interface.

4. **Storage is backend-agnostic** — `FileStorage` for local development, `ProviderStorage`
   wrapping `gokit/storage.Store` for S3/GCS/etc. Same interface.

5. **Follow gokit conventions** — Generics, functional options, interface composition,
   minimal interfaces, middleware chains.

## Module Architecture

gokit uses independent Go modules so users only import what they need. `bench` follows
the same pattern — split by **dependency boundary**, not by code size.

The rule: if a package introduces a new external dependency, it's a separate module.

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Module: github.com/kbukum/gokit/bench                                   │
│ go.mod deps: github.com/kbukum/gokit (core only — no heavy deps)        │
│                                                                         │
│ bench/                    — Core framework                              │
│ ├── doc.go                — Package documentation                       │
│ ├── types.go              — Sample[L], Prediction[L], ScoredSample[L]   │
│ ├── dataset.go            — DatasetManifest, DatasetLoader              │
│ ├── evaluator.go          — Evaluator interface, adapters               │
│ ├── runner.go             — BenchRunner orchestration                   │
│ ├── options.go            — Functional options                          │
│ ├── result.go             — RunResult, BranchResult, SampleResult       │
│ ├── curves.go             — ROCCurve, CalibrationCurve, etc.            │
│ ├── schema.go             — Bench JSON schema version                   │
│ ├── storage.go            — RunStorage interface, FileStorage           │
│ ├── compare.go            — RunComparator, regression detection         │
│ ├── middleware.go          — Timing, caching middleware                  │
│ ├── process.go            — ProcessEvaluator (gokit/process)            │
│ └── cli.go                — CLI helpers                                 │
│                                                                         │
│ bench/metric/             — All metric implementations (pure math)      │
│ ├── metric.go             — Metric[L] interface, Result, Suite          │
│ ├── classification.go     — Binary, multi-class, confusion matrix       │
│ ├── probability.go        — AUC-ROC, Brier, log loss, calibration      │
│ ├── ranking.go            — NDCG, MAP, precision@k, recall@k           │
│ ├── regression.go         — MAE, MSE, RMSE, R²                         │
│ ├── matching.go           — Exact, partial, fuzzy match                 │
│ └── composite.go          — Weighted composite                          │
│                                                                         │
│ bench/report/             — Output format reporters (stdlib only)       │
│ ├── reporter.go           — Reporter interface                          │
│ ├── json.go               — Canonical Bench JSON                        │
│ ├── markdown.go           — Markdown tables                             │
│ ├── csv.go                — Flat tabular                                │
│ ├── table.go              — Terminal-friendly                            │
│ ├── junit.go              — JUnit XML (encoding/xml)                    │
│ ├── vegalite.go           — Vega-Lite JSON specs                        │
│ └── html.go               — Self-contained HTML + embedded Vega-Embed   │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│ Module: github.com/kbukum/gokit/bench/viz                               │
│ go.mod deps: github.com/kbukum/gokit/bench + pure-Go SVG library        │
│                                                                         │
│ bench/viz/                — Image generation (optional)                 │
│ ├── doc.go                — Package documentation                       │
│ ├── render.go             — Render RunResult → SVG/PNG files            │
│ ├── confusion.go          — Confusion matrix heatmap                    │
│ ├── roc.go                — ROC curve plot                              │
│ ├── calibration.go        — Calibration curve                           │
│ ├── distribution.go       — Score distribution histogram                │
│ └── comparison.go         — Branch comparison bar chart                 │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│ Module: github.com/kbukum/gokit/bench/storage                           │
│ go.mod deps: github.com/kbukum/gokit/bench + github.com/kbukum/gokit/storage │
│                                                                         │
│ bench/storage/            — Cloud storage adapter (optional)            │
│ ├── doc.go                — Package documentation                       │
│ └── provider.go           — ProviderStorage wrapping gokit/storage.Store│
└─────────────────────────────────────────────────────────────────────────┘
```

### Why Three Modules?

| Module | External Deps | Who Needs It |
|--------|--------------|--------------|
| `bench` | gokit core only | Everyone doing benchmarking |
| `bench/viz` | Pure-Go SVG library | Teams wanting image exports |
| `bench/storage` | gokit/storage (→ AWS SDK) | Teams persisting results to S3/GCS |

This follows the exact same pattern as the rest of gokit:

```
gokit/                    ← core: zerolog, viper, otel (everyone needs)
gokit/database            ← own module: pulls in GORM (only if you use DB)
gokit/redis               ← own module: pulls in go-redis (only if you use Redis)
gokit/storage             ← own module: pulls in AWS SDK (only if you use S3)
gokit/bench               ← own module: only gokit core (only if you benchmark)
gokit/bench/viz           ← own module: pulls in SVG lib (only if you render images)
gokit/bench/storage       ← own module: pulls in gokit/storage (only if you use S3 for results)
```

### Import Examples

```go
// Core benchmarking — most users need only this
import (
    "github.com/kbukum/gokit/bench"
    "github.com/kbukum/gokit/bench/metric"
    "github.com/kbukum/gokit/bench/report"
)

// Add image generation (pulls in SVG dependency)
import "github.com/kbukum/gokit/bench/viz"

// Add S3 result storage (pulls in gokit/storage → AWS SDK)
import benchstore "github.com/kbukum/gokit/bench/storage"
```

### Dependency Graph

```
github.com/kbukum/gokit (core)
         ↑
         │ (replace ../  in local dev)
         │
github.com/kbukum/gokit/bench
    ├── bench/metric/   (sub-package, same module)
    └── bench/report/   (sub-package, same module)
         ↑                          ↑
         │                          │
github.com/kbukum/gokit/bench/viz   github.com/kbukum/gokit/bench/storage
    deps: + SVG library                 deps: + gokit/storage (→ AWS SDK)
```

No circular dependencies. Each module only pulls what it needs. Users `go get` exactly
what they use.

## Core Types

### Sample & Prediction

```go
// Sample represents a labeled data point with a generic label type.
type Sample[L comparable] struct {
    ID       string
    Input    []byte
    Label    L
    Source   string
    Metadata map[string]any
}

// Prediction is what an evaluator produces for a sample.
type Prediction[L comparable] struct {
    SampleID string
    Label    L
    Score    float64
    Scores   map[L]float64    // Per-class probabilities (optional, for multi-class)
    Metadata map[string]any
}

// ScoredSample pairs ground truth with prediction for metric computation.
type ScoredSample[L comparable] struct {
    Sample     Sample[L]
    Prediction Prediction[L]
}

// LabelMapper converts string labels from manifest files to typed labels.
type LabelMapper[L comparable] func(string) (L, error)
```

### Evaluator — Extends Provider

```go
// Evaluator is a provider.RequestResponse that produces predictions.
type Evaluator[L comparable] interface {
    provider.RequestResponse[[]byte, Prediction[L]]
}

// EvaluatorFunc wraps a plain function as an Evaluator.
func EvaluatorFunc[L comparable](name string, fn func(ctx context.Context, input []byte) (Prediction[L], error)) Evaluator[L]

// FromProvider adapts any RequestResponse provider into an Evaluator
// using mapper functions for input/output transformation.
func FromProvider[I, O any, L comparable](
    p provider.RequestResponse[I, O],
    toInput func([]byte) I,
    toPrediction func(O) Prediction[L],
) Evaluator[L]

// FromProcess creates an Evaluator that calls a subprocess via gokit/process.
func FromProcess[L comparable](
    name string,
    buildCmd func(Sample[L]) process.Command,
    parseOutput func(*process.Result) (Prediction[L], error),
) Evaluator[L]
```

Because Evaluator extends `provider.RequestResponse`, all provider capabilities apply:

```go
// Wrap with logging, metrics, tracing
eval := provider.Chain(
    provider.WithLogging[[]byte, bench.Prediction[string]](log),
    provider.WithMetrics[[]byte, bench.Prediction[string]](metrics),
    provider.WithTracing[[]byte, bench.Prediction[string]]("bench"),
)(rawEvaluator)

// Wrap with resilience
eval := resilience.WithRetry(rawEvaluator, retryPolicy)
```

### Dataset — Returns Iterator

```go
// DatasetLoader loads labeled samples from a manifest file.
type DatasetLoader[L comparable] struct { ... }

func NewDatasetLoader[L comparable](dir string, mapper LabelMapper[L], opts ...DatasetOption) *DatasetLoader[L]

// Iterator returns a pipeline.Iterator[Sample[L]] for lazy streaming.
func (d *DatasetLoader[L]) Iterator(ctx context.Context) (pipeline.Iterator[Sample[L]], error)

// Pipeline returns a pipeline.Pipeline[Sample[L]] for composition.
func (d *DatasetLoader[L]) Pipeline() *pipeline.Pipeline[Sample[L]]

// All loads all samples into memory.
func (d *DatasetLoader[L]) All(ctx context.Context) ([]Sample[L], error)

// Filter returns a new loader that only yields matching samples.
func (d *DatasetLoader[L]) Filter(fn func(Sample[L]) bool) *DatasetLoader[L]
```

Datasets compose with pipeline operators:

```go
// Filter to large files, evaluate in parallel
large := pipeline.Filter(dataset.Pipeline(), func(s bench.Sample[string]) bool {
    return len(s.Input) > 1000
})
batched := pipeline.Batch(large, 10, 5*time.Second)
```

### Metric Interface

```go
// Metric computes evaluation scores from predictions vs ground truth.
type Metric[L comparable] interface {
    Name() string
    Compute(scored []ScoredSample[L]) Result
}

// Result holds a metric computation's output.
type Result struct {
    Name   string
    Value  float64            // Primary scalar (for comparison/sorting)
    Values map[string]float64 // All computed values
    Detail any                // Metric-specific detail (confusion matrix, curve points, etc.)
}

// Suite groups multiple metrics for batch evaluation.
type Suite[L comparable] struct { ... }
func NewSuite[L comparable](metrics ...Metric[L]) *Suite[L]
func (s *Suite[L]) Compute(scored []ScoredSample[L]) []Result
```

### Built-in Metrics

| Category | Metrics | Use Case |
|----------|---------|----------|
| **Classification** | Precision, Recall, F1/Fβ, Accuracy, FPR, Confusion Matrix, Multi-class (macro/micro/weighted) | AI detection, spam filter, sentiment |
| **Probability** | AUC-ROC, Brier Score, Log Loss, Calibration Curve | Confidence calibration, risk scoring |
| **Ranking** | NDCG, MAP, Precision@k, Recall@k | Search results, recommendations |
| **Regression** | MAE, MSE, RMSE, R² | Score prediction, pricing |
| **Matching** | Exact Match, Partial Match (token overlap), Fuzzy Match | NER, translation, categorization |
| **Composite** | Weighted combination of any metrics | Custom aggregate scoring |

```go
// Binary classification with threshold sweep
metric.BinaryClassification(AIGenerated, metric.WithThreshold(0.5))
metric.ThresholdSweep(AIGenerated, nil) // 0.1 to 0.9

// Multi-class
metric.MultiClassClassification([]Sentiment{Positive, Negative, Neutral})

// Probability calibration
metric.AUCROC(AIGenerated)
metric.BrierScore(AIGenerated)

// Ranking
metric.NDCG(10)
metric.PrecisionAtK(Relevant, 5)

// Regression
metric.MAE()
metric.RMSE()

// Matching
metric.ExactMatch[string]()
metric.FuzzyMatch(0.8)

// Composite
metric.Weighted(map[metric.Metric[string]]float64{
    metric.BinaryClassification(AIGenerated): 0.7,
    metric.AUCROC(AIGenerated):               0.3,
})
```

## BenchRunner

The runner orchestrates evaluation: loads samples, runs all evaluator branches concurrently,
computes metrics, persists results.

```go
runner := bench.NewBenchRunner(
    bench.WithMetrics(
        metric.BinaryClassification(AIGenerated),
        metric.AUCROC(AIGenerated),
    ),
    bench.WithStorage(bench.NewFileStorage("results/")),
    bench.WithConcurrency(4),
    bench.WithTag("baseline-v1"),
    bench.WithTimeout(30 * time.Second),
)

runner.Register("statistical", statisticalEval)
runner.Register("binoculars", binocularsEval, bench.WithTier(2))

result, err := runner.Run(ctx, dataset)
```

### Runner Options

| Option | Purpose |
|--------|---------|
| `WithMetrics(...)` | Metric suite to compute |
| `WithStorage(s)` | Where to persist results |
| `WithConcurrency(n)` | Parallel sample evaluation (uses `pipeline.Parallel`) |
| `WithTimeout(d)` | Per-sample timeout |
| `WithTag(tag)` | Label for the run |
| `WithAggregator(fn)` | Custom multi-branch score aggregation |
| `WithObservability(tracer)` | OpenTelemetry tracing per sample |

### Internal Flow

```
                      BenchRunner.Run(ctx, dataset)
                                │
                    ┌───────────┴───────────┐
                    │  dataset.All(ctx)      │
                    │  → []Sample[L]         │
                    └───────────┬───────────┘
                                │
                    ┌───────────┴───────────┐
                    │  pipeline.Parallel     │
                    │  (n workers)           │
                    └───────────┬───────────┘
                                │
                    ┌───────────┴───────────┐
                    │  For each sample:      │
                    │  branch.Execute(ctx,   │
                    │    sample.Input)       │
                    │  → Prediction[L]       │
                    │  (with OTel span)      │
                    └───────────┬───────────┘
                                │
                    ┌───────────┴───────────┐
                    │  Aggregate branch      │
                    │  scores → overall      │
                    └───────────┬───────────┘
                                │
                    ┌───────────┴───────────┐
                    │  metric.Suite.Compute  │
                    │  → []Result            │
                    └───────────┬───────────┘
                                │
                    ┌───────────┴───────────┐
                    │  storage.Save(result)  │
                    └───────────────────────┘
```

## Storage & Comparison

### Storage

```go
// RunStorage persists benchmark results.
type RunStorage interface {
    Save(ctx context.Context, result *RunResult) (string, error)
    Load(ctx context.Context, runID string) (*RunResult, error)
    Latest(ctx context.Context) (*RunResult, error)
    List(ctx context.Context, opts ...ListOption) ([]RunSummary, error)
}

// Local filesystem (in bench core — no external deps)
bench.NewFileStorage("results/")

// S3/GCS (separate module: github.com/kbukum/gokit/bench/storage)
import benchstore "github.com/kbukum/gokit/bench/storage"
benchstore.NewProviderStorage(s3Store)
```

### Run Comparison

```go
comparator := bench.NewRunComparator()
diff := comparator.Compare(runA, runB)

fmt.Println(diff.Summary())
// ✅ F1: 0.85 → 0.91 (+0.06)
// ✅ Precision: 0.88 → 0.93 (+0.05)
// ⚠️  Recall: 0.82 → 0.80 (-0.02)
// Fixed: 5 samples | Regressed: 2 samples
```

### Reporters

```go
bench.NewMarkdownReporter().Generate(result)   // Rich markdown with tables
bench.NewJSONReporter().Generate(result)        // Machine-readable JSON
bench.NewTableReporter().Generate(result)       // Terminal-friendly
bench.NewCSVReporter().Generate(result)         // Spreadsheet-compatible
bench.NewJUnitReporter().Generate(result)       // CI/CD integration
bench.NewVegaReporter().Generate(result)        // Vega-Lite visualization specs
bench.NewHTMLReporter().Generate(result)        // Self-contained visual report
```

## Standard Output Formats

A key design goal is that bench results are **interoperable** — like how test coverage has
standardized formats (JUnit XML, Cobertura, LCOV), bench defines a structured report format
that other tools can consume. The core principle: **bench produces data + visualization specs,
rendering is a separate concern**.

This mirrors `go test -coverprofile=coverage.out` → `go tool cover -html=coverage.out`.

### Format Overview

| Format | File | Purpose | Consumer |
|--------|------|---------|----------|
| **Bench JSON** | `benchreport.json` | Primary structured output — full results, metrics, curves, samples | Dashboards, APIs, comparison tools, `bench compare` |
| **CSV** | `benchreport.csv` | Flat tabular metrics | Spreadsheets, pandas, R, data analysis |
| **JUnit XML** | `benchreport.xml` | CI/CD integration — each metric target as a test case | GitHub Actions, Jenkins, GitLab CI |
| **Markdown** | `benchreport.md` | Human-readable report | PR comments, documentation |
| **HTML** | `benchreport.html` | Self-contained visual report with embedded charts | Browser, team sharing |
| **Vega-Lite** | `charts/*.vl.json` | Declarative visualization specs | Vega Editor, VS Code, HTML embed |

### Bench JSON Schema (Primary Format)

The canonical output format. All other formats are derived from this.

```json
{
  "$schema": "https://gokit.dev/bench/v1/schema.json",
  "version": "1.0",

  "run": {
    "id": "text_statistical_diverse-v1_20260318-120000",
    "timestamp": "2026-03-18T12:00:00Z",
    "tag": "baseline-v1",
    "duration_ms": 4500
  },

  "dataset": {
    "name": "diverse-v1",
    "version": "1.0.0",
    "sample_count": 130,
    "label_distribution": { "ai_generated": 50, "human_created": 80 }
  },

  "metrics": {
    "classification": {
      "threshold": 0.5,
      "precision": 0.92,
      "recall": 0.88,
      "f1": 0.90,
      "accuracy": 0.91,
      "fpr": 0.05,
      "confusion_matrix": {
        "labels": ["ai_generated", "human_created"],
        "matrix": [[44, 6], [4, 76]],
        "orientation": "row=actual, col=predicted"
      }
    },
    "probability": {
      "auc_roc": 0.95,
      "brier_score": 0.08
    }
  },

  "curves": {
    "roc": {
      "fpr": [0.0, 0.05, 0.10, 0.20, 0.50, 1.0],
      "tpr": [0.0, 0.60, 0.85, 0.92, 0.97, 1.0],
      "thresholds": [1.0, 0.8, 0.6, 0.4, 0.2, 0.0],
      "auc": 0.95
    },
    "precision_recall": {
      "precision": [1.0, 0.95, 0.92, 0.88, 0.80],
      "recall": [0.0, 0.60, 0.75, 0.88, 1.0],
      "thresholds": [0.9, 0.7, 0.5, 0.3, 0.1]
    },
    "calibration": {
      "predicted_probability": [0.1, 0.3, 0.5, 0.7, 0.9],
      "actual_frequency": [0.12, 0.28, 0.52, 0.68, 0.91],
      "bin_count": [20, 25, 30, 28, 27]
    },
    "threshold_sweep": [
      { "threshold": 0.1, "precision": 0.70, "recall": 0.98, "f1": 0.82, "accuracy": 0.75 },
      { "threshold": 0.5, "precision": 0.92, "recall": 0.88, "f1": 0.90, "accuracy": 0.91 },
      { "threshold": 0.9, "precision": 0.99, "recall": 0.55, "f1": 0.71, "accuracy": 0.78 }
    ]
  },

  "branches": {
    "statistical": {
      "tier": 1,
      "metrics": { "f1": 0.85, "precision": 0.88, "recall": 0.82 },
      "avg_score_positive": 0.78,
      "avg_score_negative": 0.22
    },
    "binoculars": {
      "tier": 2,
      "metrics": { "f1": 0.90, "precision": 0.92, "recall": 0.88 },
      "avg_score_positive": 0.85,
      "avg_score_negative": 0.15
    }
  },

  "samples": [
    {
      "id": "ai-01",
      "label": "ai_generated",
      "predicted": "ai_generated",
      "score": 0.95,
      "correct": true,
      "branch_scores": { "statistical": 0.92, "binoculars": 0.97 },
      "duration_ms": 12
    }
  ]
}
```

### JUnit XML (CI/CD Integration)

Encode each metric target as a test case — CI/CD tools natively understand pass/fail:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="bench" tests="4" failures="1" time="4.5">
  <testsuite name="text_diverse-v1" tests="4" failures="1">
    <testcase name="f1 >= 0.85" classname="bench.classification" time="4.5">
      <!-- PASS: f1=0.90 >= 0.85 -->
    </testcase>
    <testcase name="precision >= 0.90" classname="bench.classification" time="4.5">
      <!-- PASS: precision=0.92 >= 0.90 -->
    </testcase>
    <testcase name="recall >= 0.85" classname="bench.classification" time="4.5">
      <failure message="recall=0.82 below target 0.85">
        Expected: recall >= 0.85
        Actual:   recall = 0.82 (delta: -0.03)
      </failure>
    </testcase>
    <testcase name="auc_roc >= 0.90" classname="bench.probability" time="4.5">
      <!-- PASS: auc_roc=0.95 >= 0.90 -->
    </testcase>
    <properties>
      <property name="dataset" value="diverse-v1"/>
      <property name="tag" value="baseline-v1"/>
      <property name="run_id" value="text_statistical_diverse-v1_20260318-120000"/>
    </properties>
  </testsuite>
</testsuites>
```

This means GitHub Actions, Jenkins, GitLab CI can display bench results natively in their
test result UI — just like unit tests.

### Curve Data Architecture

Metrics that produce **curve data** (not just scalar values) store their points in the
`curves` section of the JSON report. This is critical for visualization — you can't draw
an ROC curve from just the AUC number.

Curve-producing metrics:
- **ThresholdSweep** → precision/recall/f1/accuracy at each threshold
- **AUC-ROC** → FPR/TPR points
- **Precision-Recall Curve** → precision/recall points
- **Calibration Curve** → predicted probability vs actual frequency
- **Score Distribution** → histogram of scores per label

The `metric.Result` type supports this via its `Detail` field:

```go
type Result struct {
    Name   string
    Value  float64            // Primary scalar (AUC = 0.95)
    Values map[string]float64 // All scalars (precision, recall, etc.)
    Detail any                // Curve data, confusion matrix, etc.
}

// Curve-producing metrics set Detail to typed curve data:
type ROCCurve struct {
    FPR        []float64
    TPR        []float64
    Thresholds []float64
    AUC        float64
}

type ConfusionMatrixDetail struct {
    Labels      []string
    Matrix      [][]int
    Orientation string   // "row=actual, col=predicted"
}

type CalibrationCurve struct {
    PredictedProbability []float64
    ActualFrequency      []float64
    BinCount             []int
}

type ScoreDistribution struct {
    Label     string
    Bins      []float64 // Bin edges
    Counts    []int     // Counts per bin
}
```

## Visualization Architecture

Visualization is separated into two layers:

### Layer 1: Vega-Lite Specs (in bench core — zero dependencies)

The bench module generates **Vega-Lite JSON specifications** — declarative chart definitions
that contain both the data and the visualization grammar. No charting library needed.

```go
// bench generates Vega-Lite specs from curve data
specs := bench.NewVegaReporter().GenerateAll(result)
// specs["roc_curve.vl.json"]       — ROC curve
// specs["confusion_matrix.vl.json"] — Heatmap
// specs["precision_recall.vl.json"] — PR curve
// specs["calibration.vl.json"]      — Calibration curve
// specs["score_distribution.vl.json"] — Histogram
// specs["threshold_sweep.vl.json"]  — Metrics vs threshold
// specs["branch_comparison.vl.json"] — Per-branch bar chart
```

These specs can be:
- Opened in the [Vega Editor](https://vega.github.io/editor/)
- Rendered in VS Code (Vega Viewer extension)
- Embedded in HTML reports (Vega-Embed JS library)
- Rendered server-side to SVG/PNG via vl2svg/vl2png

Example generated spec (ROC curve):

```json
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "description": "ROC Curve (AUC = 0.95)",
  "width": 400,
  "height": 400,
  "layer": [
    {
      "data": { "values": [{"fpr": 0, "tpr": 0}, {"fpr": 0.05, "tpr": 0.60}, ...] },
      "mark": { "type": "line", "point": true, "tooltip": true },
      "encoding": {
        "x": { "field": "fpr", "type": "quantitative", "title": "False Positive Rate" },
        "y": { "field": "tpr", "type": "quantitative", "title": "True Positive Rate" }
      }
    },
    {
      "data": { "values": [{"x": 0, "y": 0}, {"x": 1, "y": 1}] },
      "mark": { "type": "line", "strokeDash": [4, 4], "color": "gray" },
      "encoding": {
        "x": { "field": "x", "type": "quantitative" },
        "y": { "field": "y", "type": "quantitative" }
      }
    }
  ]
}
```

### Layer 2: benchviz Sub-Module (optional — for image generation)

A separate `bench/viz` sub-package (or `benchviz` sub-module) for when you need actual
PNG/SVG images without a browser. Uses pure-Go SVG libraries (zero CGO).

```
gokit/bench/viz/           — Optional sub-package
├── render.go              — Render Vega-Lite specs to SVG
├── confusion.go           — Confusion matrix heatmap
├── roc.go                 — ROC curve
├── calibration.go         — Calibration curve
├── distribution.go        — Score distribution histogram
├── comparison.go          — Branch comparison bar chart
└── html.go                — Self-contained HTML report with embedded Vega-Lite
```

```go
// Generate SVG from bench results
svgs := viz.RenderAll(result)
// svgs["roc_curve.svg"], svgs["confusion_matrix.svg"], ...

// Generate self-contained HTML report
html := viz.HTMLReport(result)
os.WriteFile("bench-report.html", []byte(html), 0644)
```

The HTML reporter embeds Vega-Embed JS (a small library) and all the Vega-Lite specs into
a single self-contained HTML file — open it in any browser, no server needed.

### Standard Charts

| Chart | Source Data | Purpose |
|-------|-----------|---------|
| **Confusion Matrix Heatmap** | `confusion_matrix` | Visualize TP/FP/TN/FN distribution |
| **ROC Curve** | `curves.roc` | Trade-off between TPR and FPR |
| **Precision-Recall Curve** | `curves.precision_recall` | PR trade-off (better for imbalanced data) |
| **Calibration Curve** | `curves.calibration` | Are predicted probabilities reliable? |
| **Score Distribution** | `curves.score_distribution` | Separation between positive/negative classes |
| **Threshold Sweep** | `curves.threshold_sweep` | How metrics change with decision threshold |
| **Branch Comparison** | `branches` | Side-by-side F1/precision/recall per evaluator |
| **Regression Scatter** | `samples` | Predicted vs actual values (regression tasks) |
| **Run History** | Multiple runs | Metric trends over time |

### Design Rationale

Why separate data from rendering:

1. **Zero dependencies in core** — bench core stays in the root `go.mod` with no charting deps
2. **Format longevity** — JSON data outlives any rendering library
3. **Flexible consumption** — same data feeds terminal tables, HTML reports, Jupyter notebooks, Grafana dashboards
4. **Vega-Lite is a standard** — widely supported (VS Code, Observable, Jupyter, any browser), declarative, and version-controlled
5. **CI/CD native** — JUnit XML means bench results show up in test result UIs without plugins
6. **Composable** — `bench compare` reads two JSON files; no rendering needed for regression detection

## Integration Map

| gokit Module | How bench Uses It | Module Boundary |
|---|---|---|
| **provider** | Evaluator extends `RequestResponse[I, O]` — middleware, registry, manager all work | bench → gokit core |
| **pipeline** | Dataset returns `Iterator`/`Pipeline`; runner uses `Parallel` for concurrent eval | bench → gokit core |
| **process** | `FromProcess` wraps subprocess calls (Python models, external tools) | bench → gokit core |
| **storage** | `ProviderStorage` adapts `storage.Store` for S3/GCS | bench/storage → gokit/storage |
| **observability** | Runner creates OTel spans per sample; evaluator middleware records metrics | bench → gokit core |
| **component** | `BenchComponent` implements `Component` for lifecycle management | bench → gokit core |
| **bootstrap** | Works with `App.RunTask()` for CI/CD — exit non-zero if targets not met | bench → gokit core |
| **resilience** | Evaluators get retry/circuit breaker via provider middleware | bench → gokit core |
| **config** | `BenchConfig` embeds `config.ServiceConfig` for standard config loading | bench → gokit core |
| **errors** | Returns `errors.AppError` for typed error handling | bench → gokit core |

## Usage Examples

### Binary Classification (Sentinel-style)

```go
type Label string
const (AI Label = "ai_generated"; Human Label = "human_created")

dataset := bench.NewDatasetLoader("datasets/text/", func(s string) (Label, error) {
    return Label(s), nil
})

runner := bench.NewBenchRunner(
    bench.WithMetrics(metric.BinaryClassification(AI), metric.AUCROC(AI)),
    bench.WithStorage(bench.NewFileStorage("results/")),
    bench.WithConcurrency(4),
)
runner.Register("statistical", statisticalEval)
runner.Register("binoculars", binocularsEval, bench.WithTier(2))

result, _ := runner.Run(ctx, dataset)
bench.NewMarkdownReporter().Generate(result)
```

### Multi-Class Sentiment Analysis

```go
type Sentiment string
const (Pos Sentiment = "positive"; Neg Sentiment = "negative"; Neu Sentiment = "neutral")

runner := bench.NewBenchRunner(
    bench.WithMetrics(
        metric.MultiClassClassification([]Sentiment{Pos, Neg, Neu}),
        metric.ExactMatch[Sentiment](),
    ),
)
```

### Regression (Score Prediction)

```go
runner := bench.NewBenchRunner[float64](
    bench.WithMetrics(metric.MAE(), metric.RMSE(), metric.RSquared()),
)
```

### Subprocess Evaluator (Python Model)

```go
pyEval := bench.FromProcess("gpt-detector",
    func(s bench.Sample[string]) process.Command {
        return process.Command{Binary: "python", Args: []string{"-m", "detector"}, Stdin: bytes.NewReader(s.Input)}
    },
    func(r *process.Result) (bench.Prediction[string], error) {
        var out struct{ Label string; Score float64 }
        json.Unmarshal(r.Stdout, &out)
        return bench.Prediction[string]{Label: out.Label, Score: out.Score}, nil
    },
)
```

### CI/CD via Bootstrap

```go
app, _ := bootstrap.NewApp(cfg)
app.RegisterComponent(bench.NewBenchComponent(runner, dataset,
    bench.WithTargets(map[string]float64{"f1": 0.85, "precision": 0.90}),
    bench.WithFailOnRegression(true),
))
app.RunTask(ctx, nil) // Exits non-zero if targets not met
```

## Implementation Phases

### Phase 1: Foundation (bench module)

Core types, dataset loading, metric interface, and binary classification metrics. This is
the minimum viable bench — enough to replicate what pykit/bench does today.

- `go.mod` — `github.com/kbukum/gokit/bench`, depends on gokit core only
- `types.go` — Sample, Prediction, ScoredSample, LabelMapper
- `curves.go` — ROCCurve, CalibrationCurve, ScoreDistribution, ConfusionMatrixDetail
- `dataset.go` — DatasetManifest, DatasetLoader with Iterator/Pipeline support
- `evaluator.go` — Evaluator interface, EvaluatorFunc, FromProvider adapter
- `metric/metric.go` — Metric interface, Result, Suite
- `metric/classification.go` — BinaryClassification, ConfusionMatrix, ThresholdSweep

### Phase 2: Runner & Storage (bench module)

The orchestration layer that runs evaluators against datasets and persists results.

- `runner.go` — BenchRunner with concurrent evaluation
- `options.go` — Functional options
- `result.go` — RunResult, BranchResult, SampleResult
- `schema.go` — Bench JSON schema versioning and serialization
- `storage.go` — RunStorage interface, FileStorage (local filesystem, no external deps)

### Phase 3: Standard Reporters (bench/report sub-package)

Output in all standard formats — this is the "test coverage" layer.

- `report/reporter.go` — Reporter interface
- `report/json.go` — Bench JSON canonical format (with curves section)
- `report/markdown.go` — Markdown tables with confusion matrix
- `report/table.go` — Terminal-friendly output
- `report/csv.go` — Flat tabular export
- `report/junit.go` — JUnit XML for CI/CD (metric targets as test cases)

### Phase 4: Comparison & CLI (bench module)

Track improvements over time, detect regressions, terminal commands.

- `compare.go` — RunComparator with regression detection
- `cli.go` — run, compare, history commands

### Phase 5: Extended Metrics (bench/metric sub-package)

Cover the full spectrum of evaluation tasks beyond binary classification.

- `metric/probability.go` — AUC-ROC, Brier score, log loss (produce curve data)
- `metric/ranking.go` — NDCG, MAP, precision@k
- `metric/regression.go` — MAE, MSE, RMSE, R²
- `metric/matching.go` — Exact, partial, fuzzy match
- `metric/composite.go` — Weighted composite

### Phase 6: Visualization Specs & HTML (bench/report sub-package)

Vega-Lite specs (zero-dep, still in bench module) and self-contained HTML reports.

- `report/vegalite.go` — Vega-Lite JSON specs for all chart types
- `report/html.go` — Self-contained HTML with embedded Vega-Lite specs + Vega-Embed

### Phase 7: Image Generation (bench/viz module — separate go.mod)

Optional sub-module for actual PNG/SVG image rendering.

- `viz/go.mod` — `github.com/kbukum/gokit/bench/viz`, deps: bench + pure-Go SVG lib
- `viz/render.go` — Render RunResult → SVG/PNG files
- `viz/confusion.go` — Confusion matrix heatmap
- `viz/roc.go` — ROC curve plot
- `viz/calibration.go` — Calibration curve
- `viz/distribution.go` — Score distribution histogram
- `viz/comparison.go` — Branch comparison bar chart

### Phase 8: Ecosystem Integrations (bench module + bench/storage module)

Deep integration with gokit's ecosystem modules.

- `process.go` — ProcessEvaluator via gokit/process (in bench module, uses core)
- `middleware.go` — Timing, caching middleware (in bench module)
- `bootstrap.go` — BenchComponent for App lifecycle (in bench module)
- `storage/go.mod` — `github.com/kbukum/gokit/bench/storage`, deps: bench + gokit/storage
- `storage/provider.go` — ProviderStorage adapter for S3/GCS

### Phase 9: Documentation & Tests

- `doc.go` — Package documentation following gokit conventions
- `metric/doc.go`, `report/doc.go`, `viz/doc.go` — Sub-package docs
- Comprehensive test coverage for all packages
- Update root README.md module map
- CHANGELOG.md entry

## Cross-Language Portability

gokit/bench is the **reference implementation**. pykit and ruskit will implement the same
module with identical structure, naming, and semantics. This enables:

- Developers who know one kit immediately understand the others
- Same test datasets, same JSON output schema, same Vega-Lite specs across languages
- Design once in gokit, port to pykit/ruskit with language-appropriate idioms

### Mapping Table

| Concept | gokit (Go) | pykit (Python) | ruskit (Rust) |
|---------|-----------|---------------|--------------|
| **Module** | `go.mod` sub-module | `pyproject.toml` package or extras | Cargo workspace crate or feature flag |
| **Generics** | `[L comparable]` | `Generic[L]` + `TypeVar` | `<L: Eq + Hash + Clone>` |
| **Interfaces** | `interface` | `Protocol` / ABC | `trait` |
| **Functional options** | `...Option` pattern | `**kwargs` / builder | Builder pattern |
| **Provider** | `provider.RequestResponse[I,O]` | `Protocol` with `async execute(I) → O` | `trait RequestResponse<I,O>` |
| **Iterator** | `pipeline.Iterator[T]` | `AsyncIterator[T]` | `Iterator<Item=Result<T>>` |
| **Middleware** | `func(RR) RR` wrapper | Decorator / wrapper | Trait wrapper / newtype |
| **Optional deps** | Separate `go.mod` module | Optional package extras or separate package | Feature flags or separate crate |

### Module Structure (identical across languages)

```
{kit}/bench/              — Core: types, dataset, evaluator, runner, storage, compare
{kit}/bench/metric/       — All metric implementations
{kit}/bench/report/       — All output format reporters
{kit}/bench/viz           — Optional: image generation (separate module/package)
{kit}/bench/storage       — Optional: cloud storage adapter (separate module/package)
```

### Naming Convention

All public types and functions use the **same names** across languages, adapted only for
language casing convention:

| gokit | pykit | ruskit |
|-------|-------|--------|
| `Sample[L]` | `Sample[L]` | `Sample<L>` |
| `Prediction[L]` | `Prediction[L]` | `Prediction<L>` |
| `BenchRunner` | `BenchRunner` | `BenchRunner` |
| `NewBenchRunner()` | `BenchRunner()` | `BenchRunner::new()` |
| `metric.BinaryClassification()` | `metric.binary_classification()` | `metric::binary_classification()` |
| `report.NewJUnitReporter()` | `report.JUnitReporter()` | `report::JUnitReporter::new()` |

### Shared Artifacts

These are **language-independent** and identical across all three kits:

- `manifest.json` schema — dataset format
- `benchreport.json` schema — result output format
- `*.vl.json` — Vega-Lite visualization specs
- JUnit XML output — CI/CD integration
- Test datasets — same samples, same labels

### Porting Workflow

1. Design and implement in **gokit** (reference implementation)
2. Port to **pykit** — adapt to Python idioms (async/await, Pydantic, protocols)
3. Port to **ruskit** — adapt to Rust idioms (traits, serde, async with tokio)
4. Validate: all three kits produce **identical JSON output** for the same dataset
