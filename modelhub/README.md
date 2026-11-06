# modelhub

Read model metadata from public model sources without downloading model weights, and estimate the memory a model needs.

## Install

```bash
go get github.com/kbukum/gokit/modelhub@latest
```

## Packages

| Package | Reads |
|---|---|
| `modelhub/dockerhub` | Docker Hub API listings: a namespace's repositories with pull and star counts, a repository's overview, and its tags with manifest digests and download sizes. |
| `modelhub/oci` | OCI registry model artifacts, such as Docker Hub `ai/*`: manifests and their annotations, CNCF ModelPack and Docker model configs, weight and projector layer sizes, tags, digests by HEAD, and byte ranges of blobs. |
| `modelhub/huggingface` | Hugging Face Hub search, model details, file sizes, LFS digests, licenses, gating, and byte ranges of repository files. |
| `modelhub/gguf` | A GGUF header under strict limits: architecture, name, license, quantization, context length, layers and attention numbers. |
| `modelhub/memory` | The memory a model needs: its weight file size plus the key-value cache for a chosen context length. |

To download a model into a local runtime, use the runtime's adapter, such as `workload/lmstudio` (`Downloader`) or `workload/ollama`.

## Quick start

```go
hc, err := httpclient.New(httpclient.Config{
    Timeout:           30 * time.Second,
    EndpointPolicy:    policy, // grant the registry, its token realm and its blob CDN (Docker Hub: registry-1.docker.io, auth.docker.io, production.cloudfront.docker.com)
    EndpointRedirects: 3,      // blob reads redirect to a CDN
})
if err != nil {
    return err
}
registry, err := oci.New(oci.Config{HTTP: hc})
if err != nil {
    return err
}
ref, err := oci.ParseReference("ai/smollm2:360M-Q4_K_M")
if err != nil {
    return err
}
model, err := registry.Resolve(ctx, ref) // manifest + config, digests verified
if err != nil {
    return err
}
header, err := gguf.Read(ctx, registry.Blob(ref.Repository, model.Weights[0]), gguf.DefaultLimits())
if err != nil {
    return err
}
in, err := memory.FromHeader(header, model.WeightBytes)
if err != nil {
    return err
}
in.ContextLength = 8192
need, err := memory.Estimate(in)
```

`huggingface.Client.File` returns the same `gguf.Source` for a Hub file; pin the revision to `Summary.SHA` to read the file the metadata described. File reads redirect to a CDN host the Hub picks per file and region (seen: `cdn-lfs.hf.co`, `cas-bridge.xethub.hf.co`, `us.aws.cdn.hf.co`); grant the ones you need, and treat a refused redirect as "not known".

`dockerhub.Client` lists a namespace's repositories with pull and star counts, and each repository's tags with the manifest digest and download size the Hub reports. It reads the Hub API, not the registry, so listing never counts against the registry's anonymous pull limit. Use `oci.Client.Digest` (a HEAD request, also not counted as a pull) to check a tag's current digest, and resolve by digest only when you need the manifest's facts.

## Safety

- All model data is untrusted. Bodies are bounded, manifest and config digests are verified, IDs, revisions and paths are validated, and the GGUF reader never executes or renders anything a file holds.
- Every client takes an injected `httpclient.Adapter` with an endpoint policy and without a base URL or retry policy, plus an optional `util.TimerClock`; there is no global state. Only idempotent reads are retried, with bounded jittered backoff that honors `Retry-After`.
- Failures are typed `errors.AppError` values with a status detail and never carry a response body. Registry tokens are cached per client, bounded and expired on the injected clock; Hub credentials come only from the adapter's auth.
- The endpoint policy is required because sources hand out follow-up URLs (token realms, redirects); only granted hosts are reachable. Redirects are refused unless the adapter also sets `EndpointRedirects`; a followed redirect must be a granted read; a hop to another origin keeps only `Accept`, `Range` and `User-Agent` headers, while a hop on the same origin, such as a Hub file's `/api/resolve-cache` redirect, keeps the credentials a gated file needs.

## Specifications

- [OCI distribution specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)
- [CNCF ModelPack specification](https://github.com/modelpack/model-spec)
- [Hugging Face Hub API](https://huggingface.co/docs/hub/api)
- [GGUF specification](https://github.com/ggml-org/ggml/blob/master/docs/gguf.md)
