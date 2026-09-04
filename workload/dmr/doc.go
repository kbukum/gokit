// Package dmr provides a Docker Model Runner (DMR) backend for the workload
// module's runtime-neutral model runtime port.
//
// DMR ([Docker Model Runner]) manages, runs, and serves AI models via Docker and
// exposes OpenAI-, Anthropic-, and Ollama-compatible inference endpoints. This
// package adapts DMR's REST surface to [github.com/kbukum/gokit/workload.ModelRuntime]:
// [Runtime.Start] pulls a model, [Runtime.Endpoint] reports the OpenAI-compatible
// base URL to send inference to, and the optional
// [github.com/kbukum/gokit/workload.ModelLister] /
// [github.com/kbukum/gokit/workload.ModelRemover] capabilities manage local models.
//
// It is opt-in with no init side effects: construct a
// [github.com/kbukum/gokit/workload.ModelRuntimeRegistry], call [Register], then
// select the "dmr" provider through configuration. Other runtimes (Ollama, OCI,
// Hugging Face) follow the identical Register pattern behind the same port.
//
// # Quick start
//
//	reg := workload.NewModelRuntimeRegistry()
//	if err := dmr.Register(reg, dmr.Config{}); err != nil {
//		return err
//	}
//	rt, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: workload.ProviderDMR}, log)
//	if err != nil {
//		return err
//	}
//	h, err := rt.Start(ctx, workload.ModelSpec{Ref: "ai/smollm2"})
//	// send inference to h.Endpoint.BaseURL using an OpenAI-compatible client
//
// The DMR API is unauthenticated by design; do not expose it beyond a trusted
// boundary. A caller-supplied context deadline bounds a pull and takes
// precedence; a deadline-less context falls back to the configurable
// [Config.PullTimeout] (default 30m) so a stalled stream cannot hang forever.
//
// [Docker Model Runner]: https://docs.docker.com/ai/model-runner/
package dmr
