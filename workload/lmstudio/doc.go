// Package lmstudio lists and downloads the models an LM Studio server holds.
//
// [Lister.ListModels] reads GET /api/v1/models for the workload module's
// [github.com/kbukum/gokit/workload.ModelLister] port. It states each model's type, architecture, quantization,
// context length and, for language models, vision, tool-use and reasoning support. Fields LM Studio does not state
// stay unknown.
//
// [Downloader.Start] asks the server to download a catalog model or a Hugging Face repository, [Downloader.Status]
// reads a job, and [Downloader.Follow] polls it on an injected clock until it ends. Starting a download is never
// retried; status reads retry transient failures with bounded backoff. LM Studio has no API to cancel a download, so
// canceling stops following only, and the downloader does not claim the
// [github.com/kbukum/gokit/workload.ModelPuller] contract, whose cancel stops the pull.
//
// Requests run through the canonical httpclient, injected or built from [Config], can be limited by an endpoint
// policy, read bounded bodies, and fail with status-only [Error] values or typed input and format errors.
//
// See [List your models], [Download a model] and [Get download status].
//
// [List your models]: https://lmstudio.ai/docs/developer/rest/list
// [Download a model]: https://lmstudio.ai/docs/developer/rest/download
// [Get download status]: https://lmstudio.ai/docs/developer/rest/download-status
package lmstudio
