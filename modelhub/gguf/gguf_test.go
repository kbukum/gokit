package gguf_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/gguf"
)

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	app, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("error %v is not an AppError", err)
	}
	return app.Reason
}

func TestParseReadsLlamaHeader(t *testing.T) {
	t.Parallel()
	h, err := gguf.Parse(bytes.NewReader(llamaHeader()), gguf.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != 3 || h.TensorCount != 291 || h.Truncated || h.Entries != 14 {
		t.Fatalf("header frame = %+v", h)
	}
	if h.Architecture != "llama" || h.Name != "Tiny Llama" || h.License != "apache-2.0" || h.SizeLabel != "1B" {
		t.Fatalf("general facts = %+v", h)
	}
	if h.FileType == nil || *h.FileType != 15 || h.FileType.String() != "Q4_K_M" || h.Alignment != 32 {
		t.Fatalf("file type = %v alignment = %d", h.FileType, h.Alignment)
	}
	if h.ContextLength != 131072 || h.BlockCount != 16 || h.EmbeddingLength != 2048 || len(h.HeadCount) != 1 || h.HeadCount[0] != 32 {
		t.Fatalf("attention facts = %+v", h)
	}
	if len(h.HeadCountKV) != 1 || h.HeadCountKV[0] != 8 {
		t.Fatalf("kv heads = %v", h.HeadCountKV)
	}
	if !strings.Contains(h.ChatTemplate, "messages") {
		t.Fatalf("chat template = %q", h.ChatTemplate)
	}
}

func TestParseReadsPerLayerKVHeadsAndKeyLengths(t *testing.T) {
	t.Parallel()
	data := newBuilder(3, 0, 6).
		kvString("general.architecture", "gemma3").
		kvU64("gemma3.block_count", 3).
		kvI32Array("gemma3.attention.head_count", 8, 12, 16).
		kvI32Array("gemma3.attention.head_count_kv", 4, 4, 2).
		kvU32("gemma3.attention.key_length", 256).
		kvU32("gemma3.attention.value_length", 128).
		bytes()
	h, err := gguf.Parse(bytes.NewReader(data), gguf.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(h.HeadCountKV) != 3 || h.HeadCountKV[2] != 2 || len(h.HeadCount) != 3 || h.HeadCount[1] != 12 || h.KeyLength != 256 || h.ValueLength != 128 {
		t.Fatalf("header = %+v", h)
	}
}

func TestParseReadsSlidingWindow(t *testing.T) {
	t.Parallel()
	data := newBuilder(3, 0, 5).
		kvString("general.architecture", "gemma3").
		kvU32("gemma3.attention.sliding_window", 1024).
		kvU32("gemma3.attention.sliding_window_pattern", 6).
		kvU32("gemma3.attention.key_length_swa", 128).
		kvU32("gemma3.attention.value_length_swa", 64).
		bytes()
	h, err := gguf.Parse(bytes.NewReader(data), gguf.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if h.SlidingWindow != 1024 || h.SlidingWindowPeriod != 6 || h.SlidingWindowLayers != nil || h.KeyLengthSWA != 128 || h.ValueLengthSWA != 64 {
		t.Fatalf("header = %+v", h)
	}
	perLayer := newBuilder(3, 0, 3).
		kvString("general.architecture", "gemma4").
		kvU32("gemma4.attention.sliding_window", 512).
		kvBoolArray("gemma4.attention.sliding_window_pattern", true, true, false).
		bytes()
	if h, err = gguf.Parse(bytes.NewReader(perLayer), gguf.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	if h.SlidingWindowPeriod != 0 || len(h.SlidingWindowLayers) != 3 || !h.SlidingWindowLayers[0] || h.SlidingWindowLayers[2] {
		t.Fatalf("per-layer pattern = %+v", h)
	}
	wrong := newBuilder(3, 0, 2).
		kvString("general.architecture", "gemma3").
		kvStringArray("gemma3.attention.sliding_window_pattern", "yes").
		bytes()
	if _, err := gguf.Parse(bytes.NewReader(wrong), gguf.DefaultLimits()); reasonOf(t, err) != gguf.ReasonMalformed {
		t.Fatalf("string pattern = %v", err)
	}
	boolHeads := newBuilder(3, 0, 2).
		kvString("general.architecture", "llama").
		kvBoolArray("llama.attention.head_count", true).
		bytes()
	if _, err := gguf.Parse(bytes.NewReader(boolHeads), gguf.DefaultLimits()); reasonOf(t, err) != gguf.ReasonMalformed {
		t.Fatalf("boolean head counts = %v", err)
	}
}

func TestParseReadsVersion2AndBigEndian(t *testing.T) {
	t.Parallel()
	v2 := newBuilder(2, 1, 1).kvString("general.architecture", "llama").bytes()
	if h, err := gguf.Parse(bytes.NewReader(v2), gguf.DefaultLimits()); err != nil || h.Version != 2 || h.Architecture != "llama" {
		t.Fatalf("v2 = %+v, %v", h, err)
	}
	be := &builder{order: binary.BigEndian}
	be.buf.WriteString("GGUF")
	be.u32(3)
	be.u64(0)
	be.u64(1)
	be.kvU32("llama.block_count", 7)
	h, err := gguf.Parse(bytes.NewReader(be.bytes()), gguf.DefaultLimits())
	if err != nil || !h.BigEndian || h.Entries != 1 {
		t.Fatalf("big endian = %+v, %v", h, err)
	}
}

func TestParseRejectsUnsupportedFrames(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		data   []byte
		reason string
	}{
		"empty":       {nil, gguf.ReasonMalformed},
		"bad magic":   {append([]byte("GGML"), make([]byte, 20)...), gguf.ReasonBadMagic},
		"version 1":   {newBuilder(1, 0, 0).bytes(), gguf.ReasonUnsupportedVersion},
		"version 9":   {newBuilder(9, 0, 0).bytes(), gguf.ReasonUnsupportedVersion},
		"short frame": {[]byte("GGUF\x03\x00\x00\x00\x01"), gguf.ReasonMalformed},
	}
	for name, tc := range cases {
		_, err := gguf.Parse(bytes.NewReader(tc.data), gguf.DefaultLimits())
		if err == nil || reasonOf(t, err) != tc.reason {
			t.Errorf("%s: Parse() error = %v, want reason %s", name, err, tc.reason)
		}
	}
}

func TestParseRejectsMalformedEntries(t *testing.T) {
	t.Parallel()
	badBool := newBuilder(3, 0, 1)
	badBool.str("general.flag")
	badBool.u32(tBool)
	badBool.u8(2)
	badType := newBuilder(3, 0, 1)
	badType.str("general.thing")
	badType.u32(99)
	emptyKey := newBuilder(3, 0, 1)
	emptyKey.str("")
	emptyKey.u32(tUint8)
	emptyKey.u8(1)
	controlKey := newBuilder(3, 0, 1)
	controlKey.str("general.\x00name")
	controlKey.u32(tUint8)
	controlKey.u8(1)
	badElem := newBuilder(3, 0, 1)
	badElem.str("general.list")
	badElem.u32(tArray)
	badElem.u32(77)
	badElem.u64(1)
	cases := map[string][]byte{
		"bad bool":       badBool.bytes(),
		"unknown type":   badType.bytes(),
		"empty key":      emptyKey.bytes(),
		"control key":    controlKey.bytes(),
		"bad array type": badElem.bytes(),
		"duplicate key":  newBuilder(3, 0, 2).kvString("general.name", "a").kvString("general.name", "b").bytes(),
		"truncated file": newBuilder(3, 0, 3).kvString("general.name", "a").bytes(),
		"wrong type":     newBuilder(3, 0, 2).kvString("general.architecture", "llama").kvString("llama.block_count", "16").bytes(),
	}
	for name, data := range cases {
		_, err := gguf.Parse(bytes.NewReader(data), gguf.DefaultLimits())
		if err == nil || reasonOf(t, err) != gguf.ReasonMalformed {
			t.Errorf("%s: Parse() error = %v, want malformed", name, err)
		}
	}
}

func TestParseReadsEveryIntegerWidthAndSkipsOtherValues(t *testing.T) {
	t.Parallel()
	array := func(kind uint32, n uint64, payload []byte) []byte {
		return append(append(le(uint64(kind), 4), le(n, 8)...), payload...)
	}
	data := newBuilder(3, 0, 11).
		kvString("general.architecture", "llama").
		kvRaw("llama.block_count", tUint8, 16).
		kvRaw("llama.context_length", tUint16, le(4096, 2)...).
		kvRaw("llama.embedding_length", tInt16, le(2048, 2)...).
		kvRaw("llama.attention.head_count", tInt8, 32).
		kvRaw("llama.attention.key_length", tInt64, le(64, 8)...).
		kvRaw("general.name", tString, append(le(5, 8), "\xffname"...)...).
		kvRaw("general.rope_scale", tFloat64, le(0, 8)...).
		kvRaw("general.flags", tArray, array(tUint16, 2, le(0x00010002, 4))...).
		kvRaw("general.mask", tArray, array(tBool, 2, []byte{0, 1})...).
		kvRaw("general.enabled", tBool, 1).
		bytes()
	h, err := gguf.Parse(bytes.NewReader(data), gguf.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if h.BlockCount != 16 || h.ContextLength != 4096 || h.EmbeddingLength != 2048 || len(h.HeadCount) != 1 || h.HeadCount[0] != 32 || h.KeyLength != 64 {
		t.Fatalf("header = %+v", h)
	}
	if h.Name != "\uFFFDname" {
		t.Fatalf("invalid UTF-8 name = %q, want replaced", h.Name)
	}
}

func TestParseRejectsHostileFactValues(t *testing.T) {
	t.Parallel()
	llama := func(kvs uint64) *builder { return newBuilder(3, 0, kvs).kvString("general.architecture", "llama") }
	cases := map[string][]byte{
		"negative layers":       llama(2).kvRaw("llama.block_count", tInt8, 0xff).bytes(),
		"float context":         llama(2).kvF32("llama.context_length", 4096).bytes(),
		"bool file type":        llama(2).kvRaw("general.file_type", tBool, 1).bytes(),
		"invalid bool":          llama(2).kvRaw("general.file_type", tBool, 2).bytes(),
		"string head counts":    llama(2).kvStringArray("llama.attention.head_count", "32").bytes(),
		"negative head count":   llama(2).kvI32Array("llama.attention.head_count_kv", 8, -1).bytes(),
		"string head count":     llama(2).kvString("llama.attention.head_count", "32").bytes(),
		"truncated head counts": llama(2).kvRaw("llama.attention.head_count", tArray, append(le(tInt32, 4), le(4, 8)...)...).bytes(),
	}
	for name, data := range cases {
		_, err := gguf.Parse(bytes.NewReader(data), gguf.DefaultLimits())
		if err == nil || reasonOf(t, err) != gguf.ReasonMalformed {
			t.Errorf("%s: Parse() error = %v, want malformed", name, err)
		}
	}
}

func TestParseStopsAtLimitsWithPartialFacts(t *testing.T) {
	t.Parallel()
	limits := gguf.DefaultLimits()
	limits.MaxEntries = 2
	limits.MaxStringBytes = 32
	limits.MaxBytes = 4096
	hugeString := newBuilder(3, 0, 2).kvString("general.architecture", "llama")
	hugeString.str("general.name")
	hugeString.u32(tString)
	hugeString.u64(math64)
	hugeArray := newBuilder(3, 0, 2).kvString("general.architecture", "llama")
	hugeArray.str("tokenizer.ggml.scores")
	hugeArray.u32(tArray)
	hugeArray.u32(tFloat32)
	hugeArray.u64(1 << 60)
	deep := newBuilder(3, 0, 2).kvString("general.architecture", "llama")
	deep.str("general.deep")
	deep.u32(tArray)
	for range 8 {
		deep.u32(tArray)
		deep.u64(1)
	}
	deep.u32(tUint8)
	deep.u64(1)
	deep.u8(1)
	manyStrings := newBuilder(3, 0, 2).kvString("general.architecture", "llama")
	manyStrings.str("tokenizer.ggml.tokens")
	manyStrings.u32(tArray)
	manyStrings.u32(tString)
	manyStrings.u64(1 << 62)
	for range 1000 {
		manyStrings.str("tok")
	}
	cases := map[string][]byte{
		"entry count":  newBuilder(3, 0, 1<<62).kvString("general.architecture", "llama").kvU32("llama.block_count", 16).kvU32("llama.context_length", 4096).bytes(),
		"huge string":  hugeString.bytes(),
		"huge array":   hugeArray.bytes(),
		"deep nesting": deep.bytes(),
		"byte budget":  manyStrings.bytes(),
	}
	for name, data := range cases {
		h, err := gguf.Parse(bytes.NewReader(data), limits)
		if err != nil {
			t.Errorf("%s: Parse() error = %v, want truncated header", name, err)
			continue
		}
		if !h.Truncated || h.Limit == "" || h.Architecture != "llama" {
			t.Errorf("%s: header = %+v, want truncated with facts read so far", name, h)
		}
	}
}

const math64 = uint64(1) << 63

func TestParseByteBudgetStopsReading(t *testing.T) {
	t.Parallel()
	data := llamaHeader()
	limits := gguf.DefaultLimits()
	limits.MaxBytes = int64(len(data) - 10)
	src := &countingReader{r: bytes.NewReader(append(data, make([]byte, 1<<20)...))}
	h, err := gguf.Parse(src, limits)
	if err != nil || !h.Truncated || h.Limit != gguf.LimitBytes {
		t.Fatalf("Parse() = %+v, %v", h, err)
	}
	if src.n > limits.MaxBytes {
		t.Fatalf("read %d bytes past the %d byte budget", src.n, limits.MaxBytes)
	}
}

func TestParseRejectsInvalidLimits(t *testing.T) {
	t.Parallel()
	for _, limits := range []gguf.Limits{{}, {MaxBytes: 1, MaxEntries: 1, MaxStringBytes: 1, MaxArrayLen: 1, MaxDepth: 0}, {MaxBytes: -1, MaxEntries: 1, MaxStringBytes: 1, MaxArrayLen: 1, MaxDepth: 1}} {
		if _, err := gguf.Parse(bytes.NewReader(llamaHeader()), limits); err == nil {
			t.Errorf("Parse() accepted limits %+v", limits)
		}
	}
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

type sourceFunc func(ctx context.Context, n int64) (io.ReadCloser, error)

func (f sourceFunc) OpenPrefix(ctx context.Context, n int64) (io.ReadCloser, error) { return f(ctx, n) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestReadOpensBoundedPrefixAndCloses(t *testing.T) {
	t.Parallel()
	body := &trackedBody{Reader: bytes.NewReader(llamaHeader())}
	var asked int64
	h, err := gguf.Read(t.Context(), sourceFunc(func(_ context.Context, n int64) (io.ReadCloser, error) {
		asked = n
		return body, nil
	}), gguf.DefaultLimits())
	if err != nil || h.Architecture != "llama" {
		t.Fatalf("Read() = %+v, %v", h, err)
	}
	if asked != gguf.DefaultLimits().MaxBytes || !body.closed {
		t.Fatalf("Read() asked %d bytes, closed=%v", asked, body.closed)
	}
	boom := errors.New("boom")
	if _, err := gguf.Read(t.Context(), sourceFunc(func(context.Context, int64) (io.ReadCloser, error) { return nil, boom }), gguf.DefaultLimits()); !errors.Is(err, boom) {
		t.Fatalf("Read() error = %v, want source error", err)
	}
	if _, err := gguf.Read(t.Context(), nil, gguf.DefaultLimits()); err == nil {
		t.Fatal("Read() accepted a nil source")
	}
}

func TestFileTypeNames(t *testing.T) {
	t.Parallel()
	for v, want := range map[uint32]string{0: "F32", 1: "F16", 7: "Q8_0", 19: "IQ2_XXS", 32: "BF16", 38: "MXFP4_MOE", 4: "unknown(4)", 9999: "unknown(9999)"} {
		if got := gguf.FileType(v).String(); got != want {
			t.Errorf("FileType(%d) = %q, want %q", v, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add(llamaHeader())
	f.Add(newBuilder(2, 0, 1).kvI32Array("llama.attention.head_count_kv", 1, 2).bytes())
	f.Add([]byte("GGUF"))
	f.Add(newBuilder(3, 1<<62, 1<<62).bytes())
	limits := gguf.Limits{MaxBytes: 1 << 16, MaxEntries: 64, MaxStringBytes: 1024, MaxArrayLen: 4096, MaxDepth: 3}
	f.Fuzz(func(t *testing.T, data []byte) {
		src := &countingReader{r: bytes.NewReader(data)}
		h, err := gguf.Parse(src, limits)
		if src.n > limits.MaxBytes {
			t.Fatalf("read %d bytes past the budget", src.n)
		}
		if err != nil {
			if _, ok := apperr.AsAppError(err); !ok {
				t.Fatalf("untyped error %v", err)
			}
			return
		}
		if h.Entries > limits.MaxEntries || len(h.ChatTemplate) > limits.MaxStringBytes || len(h.HeadCountKV) > limits.MaxArrayLen || len(h.HeadCount) > limits.MaxArrayLen {
			t.Fatalf("header exceeded its limits: %+v", h)
		}
	})
}
