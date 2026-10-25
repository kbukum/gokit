// Package lmstudio lists the models an LM Studio server holds and describes each one for the workload module's
// [github.com/kbukum/gokit/workload.ModelLister] port.
//
// [Lister.ListModels] reads GET /api/v1/models, which states each model's type, architecture, quantization,
// context length and, for language models, vision, tool-use and reasoning support. Fields LM Studio does not state
// stay unknown. Requests run through the canonical httpclient, can be limited by an injected endpoint policy, read
// bounded bodies, and fail with status-only errors.
//
// See [List your models].
//
// [List your models]: https://lmstudio.ai/docs/developer/rest/list
package lmstudio
