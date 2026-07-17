// Package collect orchestrates dataset collection: it streams each
// [github.com/kbukum/gokit/dataset/stage.Source] through the configured
// transforms and fail-closed schema validation, records progress in a
// [github.com/kbukum/gokit/dataset/manifest.Manifest] cache, and publishes the
// collected records to each target.
package collect
