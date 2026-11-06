// Package oci reads model artifacts from an OCI distribution registry, such as Docker Hub, without downloading
// model weights.
//
// [Client.Resolve] reads a manifest by tag or digest, verifies its sha256 digest, and reads its config blob, which
// it also verifies. It understands CNCF ModelPack configs (application/vnd.cncf.model.config.v1+json) and Docker
// model configs (application/vnd.docker.ai.model.config.v0.1+json and v0.2), and reports the weight layers and
// their sizes. The returned [Model] records the manifest digest, so a later read can pin it. [Client.Tags] lists
// tags with bounded pagination, and [Client.Blob] opens a bounded byte range of a layer for the GGUF header reader.
//
// The client authenticates anonymously with the registry's bearer-token challenge and caches a bounded number of
// tokens until they expire. Registry data is untrusted: bodies are bounded, digests are verified, and failures are
// typed [github.com/kbukum/gokit/errors.AppError] values that never carry a response body.
//
// See the [OCI distribution specification] and the [ModelPack specification].
//
// [OCI distribution specification]: https://github.com/opencontainers/distribution-spec/blob/main/spec.md
// [ModelPack specification]: https://github.com/modelpack/model-spec
package oci
