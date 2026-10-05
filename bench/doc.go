// Package bench runs typed provider evaluations with finite execution and storage budgets.
//
// A Dataset exposes Describe and Iterator. BenchRunner requires WithStore, dispatches at most C evaluations with a 2C reorder window, and commits sample-major records before the summary. Cancellation waits for admitted evaluators; evaluators must honor their context. Failed runs delete their owned objects using an independent, bounded cleanup context.
//
// ObservedEvaluator optionally reports content-only TTFT and TokenUsage. Missing usage remains distinct from known zero. Failed observations retain zero predictions for quality metrics and are excluded from timing and throughput. Warmup is unscored; repeats are separate observations.
//
// RunStreaming metrics use bounded accumulators. Whole-set metrics are admitted only within Limits.MaxRetainedSamples. Timing percentiles are exact nearest ranks: small runs sort in memory; larger runs use eight storage-backed radix passes. RunResult contains bounded summaries and typed metric details, not sample records. ResultStore.Records and Observations decode persisted records lazily.
//
// EvaluationIdentity pins the dataset, metrics, judges, tokenizers, and execution settings. SubjectIdentity records what was tested and may change without invalidating comparison. RunComparator reports Eligibility separately from measurement changes; Verdict never turns incomplete or incompatible identity into a pass. CompareSamples adds bounded ID previews and exact change counts.
//
// SchemaVersion 2.0 requires a future rskit mirror before cross-kit interchange. The report, metric, llmeval, storage, viz, and testutil subpackages provide formatters, scorers, adapters, and deterministic test helpers.
package bench
