package memory_test

import (
	"math"
	"slices"
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

func TestEstimateCapsSlidingWindowLayers(t *testing.T) {
	t.Parallel()
	// Gemma 3 4B shape: 34 layers, every sixth global (5 global, 29 sliding); 4 kv heads of width 256; window 1024; 131072 tokens.
	sliding := make([]bool, 34)
	for i := range sliding {
		sliding[i] = i%6 < 5
	}
	in := memory.Input{
		FileBytes: 1, ContextLength: 131072, Layers: 34, KVHeads: []uint64{4}, KeyLength: 256, ValueLength: 256, Cache: memory.CacheF16,
		SlidingWindow: 1024, Sliding: sliding,
	}
	got, err := memory.Estimate(in)
	if err != nil {
		t.Fatal(err)
	}
	perLayerToken := int64(4 * (256 + 256) * 2)
	// 29 sliding layers keep the window plus one 512-token batch, padded to 256 cells; 5 global layers keep it all.
	want := 5*131072*perLayerToken + 29*1536*perLayerToken
	if got.KVCache != want {
		t.Fatalf("KVCache = %d, want %d", got.KVCache, want)
	}
	// Four conversations sharing a unified cache keep four windows: 4×1024+512 = 4608 cells.
	in.Sequences = 4
	if got, err = memory.Estimate(in); err != nil || got.KVCache != 5*131072*perLayerToken+29*4608*perLayerToken {
		t.Fatalf("four sequences = %+v, %v", got, err)
	}
	// A short context caps the sliding layers too, and sliding layers may use their own head widths.
	in.Sequences, in.ContextLength, in.SlidingKeyLength, in.SlidingValueLength = 0, 1000, 128, 64
	if got, err = memory.Estimate(in); err != nil || got.KVCache != 5*1000*perLayerToken+29*1000*4*(128+64)*2 {
		t.Fatalf("short context = %+v, %v", got, err)
	}
	// Per-layer head counts give the same answer as uniform ones.
	uniform := got.KVCache
	in.KVHeads = slices.Repeat([]uint64{4}, 34)
	if got, err = memory.Estimate(in); err != nil || got.KVCache != uniform {
		t.Fatalf("per-layer heads = %+v, %v, want %d", got, err, uniform)
	}
}

func TestEstimateRefusesInconsistentSlidingWindow(t *testing.T) {
	t.Parallel()
	ok := memory.Input{FileBytes: 1, ContextLength: 8, Layers: 2, KVHeads: []uint64{1}, KeyLength: 1, ValueLength: 1, Cache: memory.CacheF16}
	for name, mutate := range map[string]func(*memory.Input){
		"window without layers": func(in *memory.Input) { in.SlidingWindow = 4 },
		"layers without window": func(in *memory.Input) { in.Sliding = []bool{true, false} },
		"layer count mismatch":  func(in *memory.Input) { in.SlidingWindow, in.Sliding = 4, []bool{true} },
	} {
		in := ok
		mutate(&in)
		if _, err := memory.Estimate(in); err == nil {
			t.Errorf("%s: Estimate() guessed", name)
		}
	}
}

func TestAnImplausibleLayerCountIsRefusedBeforeAnythingIsSizedByIt(t *testing.T) {
	t.Parallel()
	h := &gguf.Header{Architecture: "gemma3", SlidingWindow: 1024, ContextLength: 8192, BlockCount: memory.MaxLayers + 1, HeadCount: []uint64{4}, KeyLength: 8, ValueLength: 8}
	if _, err := memory.FromHeader(h, 1); apperr.Normalize(err).Code != apperr.ErrCodeInvalidInput {
		t.Errorf("FromHeader(%d layers) = %v, want invalid input", h.BlockCount, err)
	}
	in := memory.Input{FileBytes: 1, ContextLength: 8, Layers: memory.MaxLayers + 1, KVHeads: []uint64{1}, KeyLength: 1, ValueLength: 1, Cache: memory.CacheF16}
	if _, err := memory.Estimate(in); apperr.Normalize(err).Code != apperr.ErrCodeInvalidInput {
		t.Errorf("Estimate(%d layers) = %v, want invalid input", in.Layers, err)
	}
	h.BlockCount = memory.MaxLayers
	if _, err := memory.FromHeader(h, 1); err != nil {
		t.Errorf("FromHeader(%d layers) = %v", h.BlockCount, err)
	}
}

func TestFromHeaderFollowsLlamaCppSlidingWindow(t *testing.T) {
	t.Parallel()
	base := func(arch string) *gguf.Header {
		return &gguf.Header{Architecture: arch, ContextLength: 8192, BlockCount: 12, HeadCount: []uint64{4}, HeadCountKV: []uint64{1}, KeyLength: 8, ValueLength: 8}
	}
	slides := func(in memory.Input) (n int) {
		for _, s := range in.Sliding {
			if s {
				n++
			}
		}
		return n
	}
	cases := map[string]struct {
		header  func() *gguf.Header
		window  uint64
		sliding int
	}{
		"llama ignores none":    {func() *gguf.Header { return base("llama") }, 0, 0},
		"gemma3 every sixth":    {func() *gguf.Header { h := base("gemma3"); h.SlidingWindow = 1024; return h }, 1024, 10},
		"gemma3 without window": {func() *gguf.Header { return base("gemma3") }, 0, 0},
		"gemma2 default window": {func() *gguf.Header { return base("gemma2") }, 4096, 6},
		"cohere2 every fourth":  {func() *gguf.Header { h := base("cohere2"); h.SlidingWindow = 4096; return h }, 4096, 9},
		"gpt-oss alternates":    {func() *gguf.Header { h := base("gpt-oss"); h.SlidingWindow = 128; return h }, 128, 6},
		"phi3 runs full":        {func() *gguf.Header { h := base("phi3"); h.SlidingWindow = 2047; return h }, 0, 0},
		"stated period wins":    {func() *gguf.Header { h := base("gemma3"); h.SlidingWindow, h.SlidingWindowPeriod = 512, 3; return h }, 512, 8},
		"stated layers win": {func() *gguf.Header {
			h := base("olmo2")
			h.SlidingWindow, h.SlidingWindowLayers = 64, make([]bool, 12)
			h.SlidingWindowLayers[1] = true
			return h
		}, 64, 1},
	}
	for name, tc := range cases {
		in, err := memory.FromHeader(tc.header(), 1)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if in.SlidingWindow != tc.window || slides(in) != tc.sliding || (tc.window > 0 && len(in.Sliding) != 12) {
			t.Errorf("%s: window %d sliding %d of %d", name, in.SlidingWindow, slides(in), len(in.Sliding))
		}
	}
	unknown := base("olmo2")
	unknown.SlidingWindow = 4096
	if _, err := memory.FromHeader(unknown, 1); err == nil {
		t.Error("an unknown sliding-window pattern was guessed")
	}
	short := base("gemma4")
	short.SlidingWindow, short.SlidingWindowLayers = 64, []bool{true}
	if _, err := memory.FromHeader(short, 1); err == nil {
		t.Error("a per-layer pattern of the wrong length was accepted")
	}
	swa := base("gemma3")
	swa.SlidingWindow, swa.KeyLengthSWA, swa.ValueLengthSWA = 1024, 16, 4
	if in, err := memory.FromHeader(swa, 1); err != nil || in.SlidingKeyLength != 16 || in.SlidingValueLength != 4 {
		t.Errorf("sliding head widths = %+v, %v", in, err)
	}
}
