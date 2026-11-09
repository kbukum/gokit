package memory

import (
	"cmp"
	"fmt"
	"math"
	"math/bits"
	"slices"

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

// MaxLayers bounds the layer count an estimate accepts, far above any real model's, so a crafted header cannot size
// per-layer state.
const MaxLayers = 4096

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

// Input holds the numbers an estimate needs. Fields up to Cache are required; the sliding-window fields are set
// together or not at all, and Sequences and Batch have defaults.
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
	// SlidingWindow is the attention window in tokens of the sliding-window layers; zero when no layer slides.
	SlidingWindow uint64
	// Sliding marks the sliding-window layers, one value per layer; empty when no layer slides.
	Sliding []bool
	// SlidingKeyLength and SlidingValueLength are the sliding-window layers' head widths; zero means KeyLength and
	// ValueLength.
	SlidingKeyLength   uint64
	SlidingValueLength uint64
	// Sequences is how many conversations share one unified cache, such as llama-server's parallel slots; each keeps
	// its own window in the sliding-window layers. Zero means one.
	Sequences uint64
	// Batch is the micro-batch in tokens a sliding-window layer keeps beyond its window; zero means llama.cpp's 512.
	Batch uint64
}

// DefaultBatch is llama.cpp's default micro-batch (n_ubatch).
const DefaultBatch = 512

// slidingPad is the cell multiple llama.cpp pads a sliding-window cache to.
const slidingPad = 256

// patterns are llama.cpp's sliding-window defaults per architecture when the header states no pattern: the period
// in which the last layer is global, and the window when the header states none. A period of zero means llama.cpp
// runs every layer with full attention even when a window is stated.
var patterns = map[string]struct{ period, window uint64 }{
	"gemma2":  {period: 2, window: 4096},
	"gemma3":  {period: 6},
	"cohere2": {period: 4},
	"gpt-oss": {period: 2},
	"phi3":    {},
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
// embedding width / head count; a width is derived only when every layer has the same head count. Sliding-window
// layers follow llama.cpp too: a stated per-layer pattern or period, else the architecture's default period; a
// stated window with no pattern this package knows fails instead of being guessed. Callers may lower ContextLength
// to the context they will run and set Sequences and Batch to the runtime's.
func FromHeader(h *gguf.Header, fileBytes int64) (Input, error) {
	if h == nil {
		return Input{}, apperr.MissingField("header")
	}
	if h.BlockCount > MaxLayers {
		return Input{}, errTooManyLayers()
	}
	in := Input{
		FileBytes: fileBytes, ContextLength: h.ContextLength, Layers: h.BlockCount, KVHeads: h.HeadCountKV, KeyLength: h.KeyLength, ValueLength: h.ValueLength,
		Cache: CacheF16, SlidingKeyLength: h.KeyLengthSWA, SlidingValueLength: h.ValueLengthSWA,
	}
	if err := slidingFromHeader(h, &in); err != nil {
		return Input{}, err
	}
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

// slidingFromHeader sets the sliding-window layers llama.cpp would use for h.
func slidingFromHeader(h *gguf.Header, in *Input) error {
	known, ok := patterns[h.Architecture]
	window := h.SlidingWindow
	if window == 0 {
		window = known.window
	}
	switch {
	case window == 0 || (ok && known.period == 0):
		return nil
	case h.SlidingWindowLayers != nil:
		if uint64(len(h.SlidingWindowLayers)) != h.BlockCount {
			return apperr.InvalidInput("attention.sliding_window_pattern", "must hold one value per layer")
		}
		in.Sliding = slices.Clone(h.SlidingWindowLayers)
	case h.SlidingWindowPeriod > 0 || ok:
		period := cmp.Or(h.SlidingWindowPeriod, known.period)
		in.Sliding = make([]bool, h.BlockCount)
		for il := range in.Sliding {
			in.Sliding[il] = uint64(il)%period < period-1
		}
	default:
		return apperr.MissingField("attention.sliding_window_pattern").WithDetail("reason", "the architecture's sliding-window pattern is not known")
	}
	in.SlidingWindow = window
	return nil
}

func errTooManyLayers() error {
	return apperr.InvalidInput("layers", fmt.Sprintf("must be at most %d", MaxLayers))
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
	case in.Layers > MaxLayers:
		return Result{}, errTooManyLayers()
	case len(in.KVHeads) == 0:
		return Result{}, apperr.MissingField("kv_heads")
	case len(in.KVHeads) != 1 && uint64(len(in.KVHeads)) != in.Layers:
		return Result{}, apperr.InvalidInput("kv_heads", "must hold one value or one value per layer")
	case in.KeyLength == 0 || in.ValueLength == 0:
		return Result{}, apperr.MissingField("head_width")
	case (in.SlidingWindow == 0) != (len(in.Sliding) == 0):
		return Result{}, apperr.InvalidInput("sliding_window", "a window and its layers are set together")
	case len(in.Sliding) != 0 && uint64(len(in.Sliding)) != in.Layers:
		return Result{}, apperr.InvalidInput("sliding", "must hold one value per layer")
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
	layer := func(heads, key, value uint64) uint64 {
		return c.add(row(c.mul(heads, key)), row(c.mul(heads, value)))
	}
	// A sliding-window layer keeps each sequence's window plus one micro-batch, padded, never more than the context.
	cells := min(in.ContextLength, padded(c.add(c.mul(in.SlidingWindow, max(in.Sequences, 1)), cmp.Or(in.Batch, DefaultBatch)), slidingPad))
	slidingKey, slidingValue := cmp.Or(in.SlidingKeyLength, in.KeyLength), cmp.Or(in.SlidingValueLength, in.ValueLength)
	full, sliding := layer(in.KVHeads[0], in.KeyLength, in.ValueLength), layer(in.KVHeads[0], slidingKey, slidingValue)
	// With uniform heads only the count of each layer kind matters; otherwise KVHeads (and Sliding) bound the loop.
	slidingLayers := uint64(0)
	for _, s := range in.Sliding {
		if s {
			slidingLayers++
		}
	}
	kv := c.add(c.mul(c.mul(full, in.Layers-slidingLayers), in.ContextLength), c.mul(c.mul(sliding, slidingLayers), cells))
	if len(in.KVHeads) > 1 {
		kv = 0
		for il, heads := range in.KVHeads {
			if len(in.Sliding) > 0 && in.Sliding[il] {
				kv = c.add(kv, c.mul(layer(heads, slidingKey, slidingValue), cells))
			} else {
				kv = c.add(kv, c.mul(layer(heads, in.KeyLength, in.ValueLength), in.ContextLength))
			}
		}
	}
	if c.overflow || kv > 1<<62 {
		return Result{}, apperr.InvalidInput("input", "estimate overflows")
	}
	total := uint64(in.FileBytes) + kv
	if total > 1<<62 {
		return Result{}, apperr.InvalidInput("input", "estimate overflows")
	}
	return Result{Weights: in.FileBytes, KVCache: int64(kv), Total: int64(total)}, nil
}

// padded rounds n up to a multiple of to, saturating instead of overflowing.
func padded(n, to uint64) uint64 {
	if n > math.MaxUint64-to {
		return math.MaxUint64
	}
	return (n + to - 1) / to * to
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
