// Package bench provides a pluggable evaluation framework for benchmarking
// providers against labeled datasets.
//
// The framework bridges gokit's provider and pipeline packages to create a
// composable evaluation workflow:
//
//   - Evaluator = provider.RequestResponse[[]byte, Prediction[L]]
//   - Dataset   = pipeline.Iterator[Sample[L]], loaded from manifest files
//   - Metrics   = pluggable scorers that consume (ground-truth, prediction) pairs
//
// # Architecture
//
// bench models evaluation as a data pipeline:
//
//	Dataset → Evaluator → ScoredSample → Metrics → Results
//
// Datasets are loaded lazily through pipeline.Iterator, so arbitrarily large
// datasets stream through memory without loading everything at once.
// Evaluators wrap any provider.RequestResponse via the FromProvider adapter,
// or use EvaluatorFunc for quick inline definitions.
// Metrics are stateless functions that receive a slice of ScoredSample and
// return scalar or structured results.
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
//	suite := metric.NewSuite(metric.BinaryClassification("positive"))
//	results := suite.Compute(scored)
//
// # Sub-packages
//
//   - metric: pluggable metric implementations (classification, confusion matrix, threshold sweep)
//   - report: result formatting and output (planned)
package bench
