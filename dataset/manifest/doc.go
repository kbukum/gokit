// Package manifest is the dataset kit's cache layer: it records per-source [SourceStats]
// and completion status in a bounded, atomically-persisted [Manifest] so a collector can skip
// or resume sources across runs.
package manifest
