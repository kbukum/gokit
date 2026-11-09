// Package memory estimates the memory a model needs to run from facts read without downloading it.
//
// [Estimate] adds the weight file size to the key-value cache a conversation of a given context length needs:
// context × Σ layers (key-value heads × (key width + value width)) × bytes per cache element. As in llama.cpp, a
// sliding-window layer keeps only each sequence's window plus one micro-batch, padded to 256 cells and never more
// than the context. [FromHeader] takes those numbers from a [github.com/kbukum/gokit/modelhub/gguf.Header],
// including which layers slide: the header's per-layer pattern or period, else llama.cpp's default for the
// architecture. Missing or inconsistent inputs, such as a window whose layer pattern is not known, fail with a
// typed error instead of a guess. Any fixed allowance for the runtime or the application is the caller's choice
// and is not added here.
package memory
