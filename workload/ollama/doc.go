// Package ollama lists the models an Ollama server holds and describes each one for the workload module's
// [github.com/kbukum/gokit/workload.ModelLister] port.
//
// [Lister.ListModels] reads GET /api/tags, then POST /api/show for each model to learn its context length and
// capabilities, a few at a time ([Config.Concurrency]). If one model's show call fails or times out, or the caller's
// deadline passes first, that model keeps only what the tag list states, and the rest are still described. Fields
// Ollama does not state stay unknown. Requests run through the canonical httpclient, can be limited by an injected
// endpoint policy, read bounded bodies, and fail with status-only errors.
//
// See the [Ollama API].
//
// [Ollama API]: https://docs.ollama.com/api
package ollama
