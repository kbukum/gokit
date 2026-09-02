// Package bench provides a pluggable evaluation framework for benchmarking providers against labeled datasets.
//
// The framework bridges gokit's provider and pipeline packages to create a composable evaluation workflow:
//
//   - Evaluator = provider.RequestResponse[[]byte, Prediction[L]]
//   - Dataset   = stream.Iterator[Sample[L]], loaded from manifest files
//   - Metrics   = pluggable scorers that consume (ground-truth, prediction) pairs
//
// # Metric phases
//
// A run computes pure, deterministic metrics ([RunMetric], typically adapted from metric.Metric) first, then I/O-backed context metrics ([RunContextMetric], adapted from metric.ContextMetric via metric.AsRunContextMetric) registered with [WithContextMetrics]. Context metrics — semantic similarity, an LLM judge — receive the run context for cancellation and are responsible for bounding their own remote calls; they may fail the run. Results merge deterministically: pure metrics in registration order, then context metrics in registration order.
//
// # Architecture
//
// bench models evaluation as a data pipeline:
//
//	Dataset → Evaluator → ScoredSample → Metrics → Results
//
// Datasets are loaded lazily through stream.Iterator, so arbitrarily large datasets stream through memory without loading everything at once. Evaluators wrap any provider.RequestResponse via the FromProvider adapter, or use EvaluatorFunc for quick inline definitions. Metrics are stateless functions that receive a slice of ScoredSample and return scalar or structured results.
//
// # Quick Start
//
//	loader := bench.NewDatasetLoader[string](dir, func(s string) (string, error) {
//	    return s, nil
//	})
//	samples, _ := loader.All(ctx)
//
//	eval := bench.EvaluatorFunc("my-model", func(ctx context.Context, input []byte) (bench.Prediction[string], error) {
//	    label, score := myModel.Predict(input)
//	    return bench.Prediction[string]{Label: label, Score: score}, nil
//	})
//
//	var scored []bench.ScoredSample[string]
//	for _, s := range samples {
//	    pred, _ := eval.Execute(ctx, s.Input)
//	    scored = append(scored, bench.ScoredSample[string]{Sample: s, Prediction: pred})
//	}
//
//	clf, err := metric.BinaryClassification("positive")
//	if err != nil {
//	    return err
//	}
//	suite := metric.NewSuite(clf)
//	results := suite.Compute(scored)
//
// # Reproducibility
//
// Every run records a [RunProvenance] on its [RunResult]: the deterministic seed and RNG algorithm, the source-control commit, the tool and host identity, and an order-independent content hash of the evaluated dataset. Provenance is gathered through an injected [ProvenanceProbe] — the default [SystemProvenanceProbe] reads host/os/arch from the runtime and the git commit best-effort from CI environment variables, while a fixed probe (see bench/testutil) supplies deterministic values for offline tests. Set the run seed with [WithSeed] and the probe with [WithProvenanceProbe]. A run ID is <tag|run>_<ts>_<uuid8>: the tag (or "run" when untagged), a compact UTC timestamp, and a unique suffix — so with a fixed clock, fixed probe, seed, and an injected [WithIDSuffix] a run's JSON is byte-identical across executions.
//
// When LLM-judge context metrics (metric.LLMJudge) score the run, provenance also records a Judges list — one entry per judge metric, in suite order, with its model, resolved backend model, prompt version, and rubric fingerprint — so a run that mixes several judges maps each judge's scores to the exact model and versioned prompt that produced them and is never silently compared across revisions or across a different resolved backend.
//
// # Sub-packages
//
//   - metric: pluggable metric implementations (classification, confusion matrix, threshold sweep)
//   - report: result formatting and output (planned)
//   - testutil: deterministic test doubles (FixedProvenanceProbe)
package bench
