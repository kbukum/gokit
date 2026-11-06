package memory_test

import (
	"math"
	"testing"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/gguf"
	"github.com/kbukum/gokit/modelhub/memory"
)

func TestEstimateAddsWeightsAndKVCache(t *testing.T) {
	t.Parallel()
	// Llama 3.2 1B: 16 layers, 8 kv heads of width 64, f16 cache, 4096 tokens.
	in := memory.Input{FileBytes: 800 << 20, ContextLength: 4096, Layers: 16, KVHeads: []uint64{8}, KeyLength: 64, ValueLength: 64, Cache: memory.CacheF16}
	got, err := memory.Estimate(in)
	if err != nil {
		t.Fatal(err)
	}
	wantKV := int64(4096 * 16 * 8 * (64 + 64) * 2)
	if got.Weights != 800<<20 || got.KVCache != wantKV || got.Total != 800<<20+wantKV {
		t.Fatalf("Estimate() = %+v, want kv %d", got, wantKV)
	}
}

func TestEstimateUsesPerLayerHeadsAndQuantizedCache(t *testing.T) {
	t.Parallel()
	in := memory.Input{FileBytes: 1, ContextLength: 32, Layers: 3, KVHeads: []uint64{4, 4, 2}, KeyLength: 32, ValueLength: 32, Cache: memory.CacheQ8_0}
	got, err := memory.Estimate(in)
	if err != nil {
		t.Fatal(err)
	}
	// q8_0 stores 32 elements in 34 bytes.
	if want := int64(32 * (4 + 4 + 2) * 64 * 34 / 32); got.KVCache != want {
		t.Fatalf("KVCache = %d, want %d", got.KVCache, want)
	}
}

func TestEstimateRefusesToGuess(t *testing.T) {
	t.Parallel()
	ok := memory.Input{FileBytes: 1, ContextLength: 1, Layers: 2, KVHeads: []uint64{1}, KeyLength: 1, ValueLength: 1, Cache: memory.CacheF16}
	cases := map[string]func(*memory.Input){
		"no file size":       func(in *memory.Input) { in.FileBytes = 0 },
		"no context":         func(in *memory.Input) { in.ContextLength = 0 },
		"no layers":          func(in *memory.Input) { in.Layers = 0 },
		"no kv heads":        func(in *memory.Input) { in.KVHeads = nil },
		"layer mismatch":     func(in *memory.Input) { in.KVHeads = []uint64{1, 1, 1} },
		"no head width":      func(in *memory.Input) { in.KeyLength = 0 },
		"unknown cache":      func(in *memory.Input) { in.Cache = "f8" },
		"overflowing sizes":  func(in *memory.Input) { in.ContextLength = math.MaxUint64 / 2 },
		"overflowing layers": func(in *memory.Input) { in.Layers = math.MaxUint64 },
	}
	for name, mutate := range cases {
		in := ok
		mutate(&in)
		if _, err := memory.Estimate(in); err == nil {
			t.Errorf("%s: Estimate() guessed", name)
		} else if _, typed := apperr.AsAppError(err); !typed {
			t.Errorf("%s: untyped error %v", name, err)
		}
	}
}

func TestFromHeaderDerivesHeadWidth(t *testing.T) {
	t.Parallel()
	h := &gguf.Header{ContextLength: 131072, BlockCount: 16, EmbeddingLength: 2048, HeadCount: []uint64{32}, HeadCountKV: []uint64{8}}
	in, err := memory.FromHeader(h, 800<<20)
	if err != nil {
		t.Fatal(err)
	}
	if in.KeyLength != 64 || in.ValueLength != 64 || in.ContextLength != 131072 || in.Layers != 16 || in.Cache != memory.CacheF16 {
		t.Fatalf("FromHeader() = %+v", in)
	}
	h.KeyLength, h.ValueLength = 128, 96
	if in, err = memory.FromHeader(h, 1); err != nil || in.KeyLength != 128 || in.ValueLength != 96 {
		t.Fatalf("FromHeader() explicit widths = %+v, %v", in, err)
	}
	for name, bad := range map[string]*gguf.Header{
		"nil":           nil,
		"no heads":      {BlockCount: 1, EmbeddingLength: 64, HeadCountKV: []uint64{1}},
		"uneven width":  {BlockCount: 1, EmbeddingLength: 65, HeadCount: []uint64{2}, HeadCountKV: []uint64{1}},
		"varying heads": {BlockCount: 2, EmbeddingLength: 64, HeadCount: []uint64{2, 4}, HeadCountKV: []uint64{1}},
	} {
		if _, err := memory.FromHeader(bad, 1); err == nil {
			t.Errorf("%s: FromHeader() guessed", name)
		}
	}
}

func TestFromHeaderDefaultsKVHeadsToHeadCount(t *testing.T) {
	t.Parallel()
	// Without head_count_kv the model has no grouped-query attention: one key-value head per attention head.
	in, err := memory.FromHeader(&gguf.Header{ContextLength: 2048, BlockCount: 2, EmbeddingLength: 64, HeadCount: []uint64{4}}, 1)
	if err != nil || len(in.KVHeads) != 1 || in.KVHeads[0] != 4 || in.KeyLength != 16 {
		t.Fatalf("FromHeader() = %+v, %v", in, err)
	}
	perLayer := &gguf.Header{ContextLength: 2048, BlockCount: 2, HeadCount: []uint64{2, 4}, KeyLength: 8, ValueLength: 8}
	if in, err = memory.FromHeader(perLayer, 1); err != nil || len(in.KVHeads) != 2 || in.KVHeads[1] != 4 {
		t.Fatalf("FromHeader() per-layer heads = %+v, %v", in, err)
	}
}

func TestEstimateRoundsQuantizedRowsPerLayer(t *testing.T) {
	t.Parallel()
	// One element per K and V row still fills a whole q4_0 block of 18 bytes each.
	in := memory.Input{FileBytes: 1, ContextLength: 1, Layers: 1, KVHeads: []uint64{1}, KeyLength: 1, ValueLength: 1, Cache: memory.CacheQ4_0}
	got, err := memory.Estimate(in)
	if err != nil || got.KVCache != 36 {
		t.Fatalf("Estimate() = %+v, %v, want 36 bytes", got, err)
	}
	// K rows of 40 elements take two q8_0 blocks; V rows of 24 take one: (68+34) bytes per layer per token.
	in = memory.Input{FileBytes: 1, ContextLength: 10, Layers: 2, KVHeads: []uint64{1, 1}, KeyLength: 40, ValueLength: 24, Cache: memory.CacheQ8_0}
	if got, err = memory.Estimate(in); err != nil || got.KVCache != 10*2*(68+34) {
		t.Fatalf("Estimate() = %+v, %v, want %d", got, err, 10*2*(68+34))
	}
}
