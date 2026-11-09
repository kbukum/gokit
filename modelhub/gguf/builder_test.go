package gguf_test

import (
	"bytes"
	"encoding/binary"
	"math"
)

// builder writes GGUF bytes for tests.
type builder struct {
	buf   bytes.Buffer
	order binary.ByteOrder
}

func newBuilder(version uint32, tensors, kvs uint64) *builder {
	b := &builder{order: binary.LittleEndian}
	b.buf.WriteString("GGUF")
	b.u32(version)
	b.u64(tensors)
	b.u64(kvs)
	return b
}

func (b *builder) u8(v uint8)   { b.buf.WriteByte(v) }
func (b *builder) u32(v uint32) { _ = binary.Write(&b.buf, b.order, v) }
func (b *builder) u64(v uint64) { _ = binary.Write(&b.buf, b.order, v) }
func (b *builder) str(s string) { b.u64(uint64(len(s))); b.buf.WriteString(s) }

const (
	tUint8   = 0
	tInt8    = 1
	tUint16  = 2
	tInt16   = 3
	tUint32  = 4
	tInt32   = 5
	tFloat32 = 6
	tBool    = 7
	tString  = 8
	tArray   = 9
	tUint64  = 10
	tInt64   = 11
	tFloat64 = 12
)

// kvRaw writes an entry of any type with its already-encoded payload.
func (b *builder) kvRaw(key string, kind uint32, payload ...byte) *builder {
	b.str(key)
	b.u32(kind)
	b.buf.Write(payload)
	return b
}

// le encodes v little-endian in n bytes.
func le(v uint64, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(v >> (8 * i))
	}
	return out
}

func (b *builder) kvString(key, val string) *builder {
	b.str(key)
	b.u32(tString)
	b.str(val)
	return b
}

func (b *builder) kvU32(key string, v uint32) *builder {
	b.str(key)
	b.u32(tUint32)
	b.u32(v)
	return b
}

func (b *builder) kvU64(key string, v uint64) *builder {
	b.str(key)
	b.u32(tUint64)
	b.u64(v)
	return b
}

func (b *builder) kvF32(key string, v float32) *builder {
	b.str(key)
	b.u32(tFloat32)
	b.u32(math.Float32bits(v))
	return b
}

func (b *builder) kvI32Array(key string, vals ...int32) *builder {
	b.str(key)
	b.u32(tArray)
	b.u32(tInt32)
	b.u64(uint64(len(vals)))
	for _, v := range vals {
		b.u32(uint32(v))
	}
	return b
}

func (b *builder) kvBoolArray(key string, vals ...bool) *builder {
	b.str(key)
	b.u32(tArray)
	b.u32(tBool)
	b.u64(uint64(len(vals)))
	for _, v := range vals {
		if v {
			b.u8(1)
		} else {
			b.u8(0)
		}
	}
	return b
}

func (b *builder) kvStringArray(key string, vals ...string) *builder {
	b.str(key)
	b.u32(tArray)
	b.u32(tString)
	b.u64(uint64(len(vals)))
	for _, v := range vals {
		b.str(v)
	}
	return b
}

func (b *builder) bytes() []byte { return b.buf.Bytes() }

// llamaHeader is a small, valid header shaped like a llama.cpp export.
func llamaHeader() []byte {
	return newBuilder(3, 291, 14).
		kvString("general.architecture", "llama").
		kvString("general.name", "Tiny Llama").
		kvString("general.license", "apache-2.0").
		kvString("general.size_label", "1B").
		kvU32("general.file_type", 15).
		kvU32("general.alignment", 32).
		kvU32("llama.context_length", 131072).
		kvU32("llama.block_count", 16).
		kvU32("llama.embedding_length", 2048).
		kvU32("llama.attention.head_count", 32).
		kvU32("llama.attention.head_count_kv", 8).
		kvF32("llama.rope.freq_base", 500000).
		kvStringArray("tokenizer.ggml.tokens", "<s>", "</s>", "hello").
		kvString("tokenizer.chat_template", "{% for m in messages %}{{ m.content }}{% endfor %}").
		bytes()
}
