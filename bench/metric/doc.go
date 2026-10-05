// Package metric provides versioned evaluation metrics with typed results.
//
// Metric and ContextMetric expose Identity() bench.MetricIdentity rather than a name alone. Identity keeps canonical names, algorithm versions, and deterministic configuration separate. Result is an alias of bench.MetricResult, including typed judge, confusion, ROC, calibration, threshold-sweep, and component results.
//
// ExactMatch, FuzzyMatch, BLEU, ROUGE1, ROUGE2, ROUGEL, MAE, MSE, and RMSE implement Streaming. Each branch gets an independent accumulator. Other metrics use a storage-backed retained-observation pass subject to bench.Limits.MaxRetainedSamples.
//
// BLEU follows sacreBLEU 2.4.3 corpus BLEU with 13a tokenization, exponential smoothing, four orders, no effective order, case sensitivity, and a 0-1 scale. ROUGE follows rouge-score 0.1.2 without stemming: mean per-sample F and mean precision/recall. Pinned Python 3.11.6 reference cases use an absolute tolerance of 1e-9.
//
// Classification, probability, ranking, regression, matching, and weighted metrics remain available. SemanticSimilarity and LLMJudge are context metrics with bounded provider calls. Judge identity is recorded in Result.Judge; configuration belongs in Identity().Config, not measured Values. AsRunMetric preserves optional streaming capabilities; AsRunContextMetric adapts I/O-backed metrics.
package metric
