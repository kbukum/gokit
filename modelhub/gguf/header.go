package gguf

import (
	"fmt"
	"strings"

	apperr "github.com/kbukum/gokit/errors"
)

// Failure reasons carried by [github.com/kbukum/gokit/errors.AppError].
const (
	ReasonBadMagic           = "GGUF_BAD_MAGIC"
	ReasonUnsupportedVersion = "GGUF_UNSUPPORTED_VERSION"
	ReasonMalformed          = "GGUF_MALFORMED"
)

// Limit names reported in [Header.Limit].
const (
	LimitBytes   = "bytes"
	LimitEntries = "entries"
	LimitString  = "string"
	LimitArray   = "array"
	LimitDepth   = "depth"
)

// Limits bound how much of an untrusted header is read. Every field must be positive.
type Limits struct {
	// MaxBytes bounds the bytes read from the start of the file.
	MaxBytes int64
	// MaxEntries bounds the metadata entries read.
	MaxEntries int
	// MaxStringBytes bounds each key and string value.
	MaxStringBytes int
	// MaxArrayLen bounds each array whose values are kept, such as per-layer attention heads. Other arrays are
	// skipped within MaxBytes.
	MaxArrayLen int
	// MaxDepth bounds array nesting.
	MaxDepth int
}

// DefaultLimits reads at most 8 MiB, 4096 entries, 64 KiB per string, 4096 kept array values and 3 nested arrays.
func DefaultLimits() Limits {
	return Limits{MaxBytes: 8 << 20, MaxEntries: 4096, MaxStringBytes: 64 << 10, MaxArrayLen: 4096, MaxDepth: 3}
}

// Validate rejects a non-positive limit.
func (l Limits) Validate() error {
	if l.MaxBytes <= 0 || l.MaxEntries <= 0 || l.MaxStringBytes <= 0 || l.MaxArrayLen <= 0 || l.MaxDepth <= 0 {
		return apperr.InvalidInput("limits", "Every GGUF read limit must be positive")
	}
	return nil
}

// Header holds the facts read from a GGUF header. Zero values and nil mean unknown.
type Header struct {
	// Version is the GGUF format version (2 or 3).
	Version uint32
	// BigEndian reports a big-endian file.
	BigEndian bool
	// TensorCount is the declared number of tensors.
	TensorCount uint64
	// DeclaredEntries is the declared number of metadata entries.
	DeclaredEntries uint64
	// Entries is the number of metadata entries read.
	Entries int

	// Architecture is general.architecture, such as "llama".
	Architecture string
	// Name is general.name.
	Name string
	// License is general.license, usually an SPDX identifier.
	License string
	// SizeLabel is general.size_label, such as "8B".
	SizeLabel string
	// FileType is general.file_type, the predominant quantization.
	FileType *FileType
	// Alignment is general.alignment.
	Alignment uint64

	// ContextLength is the trained context length in tokens.
	ContextLength uint64
	// BlockCount is the number of transformer layers.
	BlockCount uint64
	// EmbeddingLength is the embedding width.
	EmbeddingLength uint64
	// HeadCount is the number of attention heads: one value for every layer, or one value per layer.
	HeadCount []uint64
	// HeadCountKV is the number of key-value heads: one value for every layer, or one value per layer.
	HeadCountKV []uint64
	// KeyLength is the key head width; zero means EmbeddingLength/HeadCount.
	KeyLength uint64
	// ValueLength is the value head width; zero means EmbeddingLength/HeadCount.
	ValueLength uint64

	// ChatTemplate is tokenizer.chat_template as plain text. It is never rendered or executed.
	ChatTemplate string

	// Truncated reports that a limit stopped reading before the metadata section ended.
	Truncated bool
	// Limit names the limit that stopped reading.
	Limit string
}

// FileType is the llama.cpp file type enumeration stored in general.file_type.
type FileType uint32

// String names the file type, or "unknown(N)".
func (t FileType) String() string {
	// Names follow llama.cpp's llama_ftype; removed values (4-6, 33-35) stay unknown.
	const known = "F32 F16 Q4_0 Q4_1 - - - Q8_0 Q5_0 Q5_1 Q2_K Q3_K_S Q3_K_M Q3_K_L Q4_K_S Q4_K_M Q5_K_S Q5_K_M Q6_K " +
		"IQ2_XXS IQ2_XS Q2_K_S IQ3_XS IQ3_XXS IQ1_S IQ4_NL IQ3_S IQ3_M IQ2_S IQ2_M IQ4_XS IQ1_M BF16 - - - TQ1_0 TQ2_0 MXFP4_MOE"
	names := strings.Fields(known)
	if int64(t) < int64(len(names)) && names[t] != "-" {
		return names[t]
	}
	return fmt.Sprintf("unknown(%d)", uint32(t))
}
