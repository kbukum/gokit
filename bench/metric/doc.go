// Package metric provides pluggable evaluation metrics for the bench framework.
//
// Metrics consume slices of [bench.ScoredSample] (ground-truth / prediction pairs) and return a [Result] containing scalar values and optional structured detail (confusion matrices, ROC curves, calibration data, etc.).
//
// # Metric Interface
//
// Every metric implements [Metric][L]:
//
//	type Metric[L comparable] interface {
//	    Name()    string
//	    Compute([]bench.ScoredSample[L]) Result
//	}
//
// Use [NewSuite] to group metrics and evaluate them in a single call.
//
// # Context metrics
//
// I/O-backed metrics — those that embed text or call an LLM judge — implement [ContextMetric][L] instead, taking a [context.Context] for cancellation and returning an error:
//
//	type ContextMetric[L comparable] interface {
//	    Name()    string
//	    Compute(context.Context, []bench.ScoredSample[L]) (Result, error)
//	}
//
// The runner computes pure [Metric]s first, then [ContextMetric]s, via [bench.WithContextMetrics] and the [AsRunContextMetric] adapter. A resolved context-metric result can be surfaced as a pure [Metric] with [AsSync] (the precompute path), so both offline and I/O-backed results can share a [Suite].
//
// # Available Metrics
//
// Classification:
//   - [BinaryClassification] — precision, recall, F1, accuracy, FPR
//   - [MultiClassClassification] — macro / micro / weighted averages
//   - [ConfusionMatrix] — full N×N confusion matrix
//   - [ThresholdSweep] — classification metrics at multiple decision thresholds
//
// Probability:
//   - [AUCROC] — Area Under ROC Curve with stored ROC data
//   - [BrierScore] — mean squared probability error
//   - [LogLoss] — logarithmic loss / cross-entropy
//   - [Calibration] — expected calibration error with bin data
//
// Ranking:
//   - [NDCG] — Normalized Discounted Cumulative Gain at k
//   - [MAP] — Mean Average Precision
//   - [PrecisionAtK] — precision in the top-k results
//   - [RecallAtK] — recall in the top-k results
//
// Regression:
//   - [MAE] — Mean Absolute Error
//   - [MSE] — Mean Squared Error
//   - [RMSE] — Root Mean Squared Error
//   - [RSquared] — coefficient of determination (R²)
//
// Matching:
//   - [ExactMatch] — fraction of exact label matches
//   - [FuzzyMatch] — Levenshtein-based string similarity
//
// Token usage:
//   - [TokenStats] — total / average token count of predictions and references
//     via an injected [llm.TokenCounter]; constructed fallibly (rejects a nil
//     counter) and marked descriptive so run comparison excludes it from
//     regression classification
//
// Semantic (context metric, I/O-backed):
//   - [SemanticSimilarity] — mean embedding cosine similarity of prediction vs
//     reference via an injected [embedding.Provider] and [vector.CosineSimilarity],
//     with a threshold match rate; constructed fallibly (rejects a nil provider),
//     every embedding call bounded by a timeout and honoring cancellation
//
// Composite:
//   - [Weighted] — weighted combination of multiple metrics
//
// # Adapter
//
// [AsRunMetric] and [AsRunMetrics] convert metric.Metric[L] values into bench.RunMetric[L] for use with [bench.BenchRunner]:
//
//	runner := bench.NewBenchRunner(
//	    bench.WithMetrics(metric.AsRunMetrics(
//	        metric.BinaryClassification[string]("positive"),
//	        metric.AUCROC[string]("positive"),
//	    )...),
//	)
//
// # Usage
//
//	suite := metric.NewSuite(
//	    metric.BinaryClassification[string]("positive"),
//	    metric.AUCROC[string]("positive"),
//	    metric.BrierScore[string]("positive"),
//	)
//	results := suite.Compute(scored)
//	for _, r := range results {
//	    fmt.Printf("%s: %.4f\n", r.Name, r.Value)
//	}
package metric
