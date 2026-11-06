package memory

import (
	"math/bits"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/gguf"
)

// Cache is the key-value cache element type.
type Cache string

// Cache element types and their storage cost.
const (
	// CacheF16 stores each element in 2 bytes, the llama.cpp default.
	CacheF16 Cache = "f16"
	// CacheF32 stores each element in 4 bytes.
	CacheF32 Cache = "f32"
	// CacheQ8_0 stores 32 elements in 34 bytes.
	CacheQ8_0 Cache = "q8_0"
	// CacheQ4_0 stores 32 elements in 18 bytes.
	CacheQ4_0 Cache = "q4_0"
)

// block returns the cache type's block: elements per block and bytes per block.
func (c Cache) block() (elements, size uint64) {
	switch c {
	case CacheF16:
		return 1, 2
	case CacheF32:
		return 1, 4
	case CacheQ8_0:
		return 32, 34
	case CacheQ4_0:
		return 32, 18
	default:
		return 0, 0
	}
}

// Input holds the numbers an estimate needs. Every field is required.
type Input struct {
	// FileBytes is the size of the weight files.
	FileBytes int64
	// ContextLength is the conversation length in tokens the runtime reserves.
	ContextLength uint64
	// Layers is the number of transformer layers.
	Layers uint64
	// KVHeads is the number of key-value heads: one value for every layer, or one value per layer.
	KVHeads []uint64
	// KeyLength is the key head width.
	KeyLength uint64
	// ValueLength is the value head width.
	ValueLength uint64
	// Cache is the cache element type.
	Cache Cache
}

// Result is an estimate in bytes.
type Result struct {
	// Weights is the weight file size.
	Weights int64
	// KVCache is the conversation memory at the requested context length.
	KVCache int64
	// Total is Weights plus KVCache.
	Total int64
}

// FromHeader builds an input from a GGUF header, the weight file size, the header's trained context length and an
// f16 cache. As in llama.cpp, key-value heads default to the attention head count and head widths default to
// embedding width / head count; a width is derived only when every layer has the same head count. Callers may
// lower ContextLength to the context they will run.
func FromHeader(h *gguf.Header, fileBytes int64) (Input, error) {
	if h == nil {
		return Input{}, apperr.MissingField("header")
	}
	in := Input{FileBytes: fileBytes, ContextLength: h.ContextLength, Layers: h.BlockCount, KVHeads: h.HeadCountKV, KeyLength: h.KeyLength, ValueLength: h.ValueLength, Cache: CacheF16}
	if len(in.KVHeads) == 0 {
		in.KVHeads = h.HeadCount
	}
	if in.KeyLength == 0 || in.ValueLength == 0 {
		heads, uniform := uniformValue(h.HeadCount)
		if !uniform || heads == 0 || h.EmbeddingLength == 0 || h.EmbeddingLength%heads != 0 {
			return Input{}, apperr.MissingField("attention.key_length").WithDetail("reason", "head width is not stated and cannot be derived")
		}
		width := h.EmbeddingLength / heads
		if in.KeyLength == 0 {
			in.KeyLength = width
		}
		if in.ValueLength == 0 {
			in.ValueLength = width
		}
	}
	return in, nil
}

// uniformValue returns the single value every element of v holds.
func uniformValue(v []uint64) (uint64, bool) {
	if len(v) == 0 {
		return 0, false
	}
	for _, x := range v[1:] {
		if x != v[0] {
			return 0, false
		}
	}
	return v[0], true
}

// Estimate returns the weight size plus the key-value cache for in. It fails instead of guessing when a number is
// missing, inconsistent or overflows.
func Estimate(in Input) (Result, error) {
	switch {
	case in.FileBytes <= 0:
		return Result{}, apperr.MissingField("file_bytes")
	case in.ContextLength == 0:
		return Result{}, apperr.MissingField("context_length")
	case in.Layers == 0:
		return Result{}, apperr.MissingField("layers")
	case len(in.KVHeads) == 0:
		return Result{}, apperr.MissingField("kv_heads")
	case len(in.KVHeads) != 1 && uint64(len(in.KVHeads)) != in.Layers:
		return Result{}, apperr.InvalidInput("kv_heads", "must hold one value or one value per layer")
	case in.KeyLength == 0 || in.ValueLength == 0:
		return Result{}, apperr.MissingField("head_width")
	}
	blockElems, blockBytes := in.Cache.block()
	if blockElems == 0 {
		return Result{}, apperr.InvalidInput("cache", "must be f16, f32, q8_0 or q4_0")
	}
	// Each layer stores one K row and one V row per token, and a quantized row fills whole blocks.
	var c checked
	row := func(elements uint64) uint64 {
		return c.mul(c.add(elements, blockElems-1)/blockElems, blockBytes)
	}
	layer := func(heads uint64) uint64 {
		return c.add(row(c.mul(heads, in.KeyLength)), row(c.mul(heads, in.ValueLength)))
	}
	var perToken uint64
	if len(in.KVHeads) == 1 {
		perToken = c.mul(layer(in.KVHeads[0]), in.Layers)
	} else {
		for _, heads := range in.KVHeads {
			perToken = c.add(perToken, layer(heads))
		}
	}
	kv := c.mul(perToken, in.ContextLength)
	if c.overflow || kv > 1<<62 {
		return Result{}, apperr.InvalidInput("input", "estimate overflows")
	}
	total := uint64(in.FileBytes) + kv
	if total > 1<<62 {
		return Result{}, apperr.InvalidInput("input", "estimate overflows")
	}
	return Result{Weights: in.FileBytes, KVCache: int64(kv), Total: int64(total)}, nil
}

// checked accumulates overflow across arithmetic.
type checked struct{ overflow bool }

func (c *checked) add(a, b uint64) uint64 {
	s, carry := bits.Add64(a, b, 0)
	c.overflow = c.overflow || carry != 0
	return s
}

func (c *checked) mul(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	c.overflow = c.overflow || hi != 0
	return lo
}
