// Package modelhub reads model metadata from public model sources without downloading model weights.
//
// The sub-packages each own one concern:
//
//   - [github.com/kbukum/gokit/modelhub/oci] reads model artifacts from an OCI registry: manifests, CNCF ModelPack
//     and Docker model configs, tags, and byte ranges of weight blobs.
//   - [github.com/kbukum/gokit/modelhub/huggingface] searches the Hugging Face Hub and reads model details, file
//     sizes, licenses and gating, and byte ranges of repository files.
//   - [github.com/kbukum/gokit/modelhub/gguf] parses a GGUF header under strict limits and never executes anything
//     the file holds.
//   - [github.com/kbukum/gokit/modelhub/memory] estimates the memory a model needs from its file size and the
//     attention numbers in its header.
//
// Every remote read goes through an injected [github.com/kbukum/gokit/httpclient.Adapter], reads a bounded body,
// retries only idempotent reads with bounded backoff on an injected clock, and fails with a typed
// [github.com/kbukum/gokit/errors.AppError] that never carries a response body. All model data is untrusted input.
package modelhub
