// Package memory estimates the memory a model needs to run from facts read without downloading it.
//
// [Estimate] adds the weight file size to the key-value cache a conversation of a given context length needs:
// context × Σ layers (key-value heads × (key width + value width)) × bytes per cache element. [FromHeader] takes
// those numbers from a [github.com/kbukum/gokit/modelhub/gguf.Header]. Missing or inconsistent inputs fail with a
// typed error instead of a guess. Any fixed allowance for the runtime or the application is the caller's choice
// and is not added here.
package memory
