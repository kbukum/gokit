// Package storage adapts gokit/storage.Storage to bench.ObjectStore. ResultStore owns run quotas, ordered record segments, cleanup, and summary commit markers; this adapter owns namespacing and bounded reads, preserving backend and reader-close errors.
package storage
