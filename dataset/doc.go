// Package dataset is a streaming dataset-collection toolkit: it fetches records from heterogeneous sources, transforms and validates them, caches progress, and publishes to configurable targets — mirroring rskit's dataset kit as a light, generics-first Go port.
//
// It is split into focused sub-packages by concern:
//
//   - [github.com/kbukum/gokit/dataset/payload] — bounded in-memory or
//     file-backed byte payloads and the resource Limits that bound them.
//   - [github.com/kbukum/gokit/dataset/record] — the tabular record.Record
//     value plus CSV/JSON-array/JSON-lines readers, writers, stream filters, and
//     the file source/target that flow records through the collector.
//   - [github.com/kbukum/gokit/dataset/sample] — the blob item family: a
//     labeled, offset-carrying sample.Item over a bounded payload, with
//     directory/slice sources and a real/AI-splitting local target.
//   - [github.com/kbukum/gokit/dataset/schema] — fail-closed JSON Schema
//     validation of records, wrapping the canonical schema validator.
//   - [github.com/kbukum/gokit/dataset/stage] — the generic streaming stages:
//     stage.Source, stage.Transform, and stage.Target over
//     [github.com/kbukum/gokit/stream] pipelines.
//   - [github.com/kbukum/gokit/dataset/manifest] — the bounded, atomically
//     persisted cache that lets a run skip or resume sources.
//   - [github.com/kbukum/gokit/dataset/collect] — the generic
//     collect.Collector[T] engine that streams sources through a bounded worker
//     pool, applies transforms and a pluggable validator, classifies items
//     real/AI, caches and resumes progress, and publishes to the targets.
package dataset
