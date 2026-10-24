// Package openaicompat lists the models an OpenAI-compatible server offers, such as vLLM, llama.cpp's server or a
// hosted API, for the workload module's [github.com/kbukum/gokit/workload.ModelLister] port.
//
// [Lister.ListModels] reads GET {base}/models. The OpenAI shape states only an id, so the descriptor is mostly
// unknown; vLLM's max_model_len becomes the context length. Requests run through the canonical httpclient, can be
// limited by an injected endpoint policy, read bounded bodies, and fail with status-only errors.
//
// See [List models] and vLLM's [ModelCard].
//
// [List models]: https://platform.openai.com/docs/api-reference/models/list
// [ModelCard]: https://github.com/vllm-project/vllm/blob/main/vllm/entrypoints/openai/protocol.py
package openaicompat
