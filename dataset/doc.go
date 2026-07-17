// Package dataset is a streaming dataset-collection toolkit: it fetches records
// from heterogeneous sources, transforms and validates them, caches progress,
// and publishes to configurable targets — mirroring rskit's dataset kit as a
// light, generics-first Go port.
//
// It is split into focused sub-packages by concern:
//
//   - [github.com/kbukum/gokit/dataset/payload] — bounded in-memory or
//     file-backed byte payloads and the resource Limits that bound them.
//   - [github.com/kbukum/gokit/dataset/record] — the tabular record.Record
//     value plus CSV/JSON-array/JSON-lines readers, writers, and stream filters.
//   - [github.com/kbukum/gokit/dataset/schema] — fail-closed JSON Schema
//     validation of records, wrapping the canonical schema validator.
//   - [github.com/kbukum/gokit/dataset/stage] — the generic streaming stages:
//     stage.Source, stage.Transform, and stage.Target over
//     [github.com/kbukum/gokit/stream] pipelines.
//   - [github.com/kbukum/gokit/dataset/manifest] — the bounded, atomically
//     persisted cache that lets a run skip or resume sources.
//   - [github.com/kbukum/gokit/dataset/collect] — the collect.Collector that
//     orchestrates sources, transforms, validation, caching, and publishing.
package dataset
