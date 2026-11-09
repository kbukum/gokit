// Package gguf reads the metadata header of a GGUF model file under strict limits.
//
// [Parse] reads the fixed frame and the key-value metadata section and stops before the tensor data. It extracts
// the facts a model catalog shows and sizes a model by: architecture, name, license, size label, quantization (file type),
// context length, layer count and the attention numbers a memory estimate needs (including the sliding window and
// which layers use it), and the chat template as plain
// text. Nothing the file holds is ever executed or rendered.
//
// The file is untrusted input. [Limits] bound the bytes read, the number of entries, each string, each retained
// array and array nesting. Reaching a limit stops reading and returns the facts read so far with
// [Header.Truncated] set; facts not reached stay unknown. Malformed input fails with a typed
// [github.com/kbukum/gokit/errors.AppError] whose reason is [ReasonBadMagic], [ReasonUnsupportedVersion] or
// [ReasonMalformed]. [Read] opens a bounded prefix through a [Source], such as an HTTP range request.
//
// See the [GGUF specification].
//
// [GGUF specification]: https://github.com/ggml-org/ggml/blob/master/docs/gguf.md
package gguf
