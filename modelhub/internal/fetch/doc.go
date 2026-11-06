// Package fetch performs the bounded, retried, idempotent reads that the modelhub source clients share. It maps
// transport failures to status-only typed errors and never exposes a response body in an error.
package fetch
