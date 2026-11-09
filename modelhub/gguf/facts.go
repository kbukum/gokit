package gguf

import (
	"errors"
	"strings"
)

const (
	keyArchitecture = "general.architecture"
	keyName         = "general.name"
	keyLicense      = "general.license"
	keySizeLabel    = "general.size_label"
	keyFileType     = "general.file_type"
	keyAlignment    = "general.alignment"
	keyChatTemplate = "tokenizer.chat_template"

	suffixContextLength   = ".context_length"
	suffixBlockCount      = ".block_count"
	suffixEmbeddingLength = ".embedding_length"
	suffixHeadCount       = ".attention.head_count"
	suffixHeadCountKV     = ".attention.head_count_kv"
	suffixKeyLength       = ".attention.key_length"
	suffixValueLength     = ".attention.value_length"
	suffixSlidingWindow   = ".attention.sliding_window"
	suffixSlidingPattern  = ".attention.sliding_window_pattern"
	suffixKeyLengthSWA    = ".attention.key_length_swa"
	suffixValueLengthSWA  = ".attention.value_length_swa"
)

var architectureSuffixes = [...]string{
	suffixContextLength, suffixBlockCount, suffixEmbeddingLength, suffixHeadCount, suffixHeadCountKV, suffixKeyLength, suffixValueLength,
	suffixSlidingWindow, suffixSlidingPattern, suffixKeyLengthSWA, suffixValueLengthSWA,
}

// wanted reports whether a key may carry a fact. Architecture keys are matched by suffix because the architecture
// entry can come after them; apply keeps only the ones under the declared architecture.
func wanted(key string) bool {
	switch key {
	case keyArchitecture, keyName, keyLicense, keySizeLabel, keyFileType, keyAlignment, keyChatTemplate:
		return true
	}
	for _, suffix := range architectureSuffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

// keepsArray reports whether a key's integer or boolean array is kept: head counts and the sliding-window pattern
// may be stated per layer.
func keepsArray(key string) bool {
	return strings.HasSuffix(key, suffixHeadCount) || strings.HasSuffix(key, suffixHeadCountKV) || strings.HasSuffix(key, suffixSlidingPattern)
}

func (h *Header) apply(facts map[string]value) error {
	var err error
	str := func(key string, dst *string) {
		if v, ok := facts[key]; ok && err == nil {
			if v.kind != typeString {
				err = malformed(errors.New(key + " is not a string"))
				return
			}
			*dst = v.str
		}
	}
	count := func(key string, dst *uint64) {
		if v, ok := facts[key]; ok && err == nil {
			if !v.isInt || v.neg {
				err = malformed(errors.New(key + " is not a non-negative integer"))
				return
			}
			*dst = v.num
		}
	}
	str(keyArchitecture, &h.Architecture)
	str(keyName, &h.Name)
	str(keyLicense, &h.License)
	str(keySizeLabel, &h.SizeLabel)
	str(keyChatTemplate, &h.ChatTemplate)
	count(keyAlignment, &h.Alignment)
	if _, ok := facts[keyFileType]; ok {
		var ft uint64
		count(keyFileType, &ft)
		if err == nil && ft <= 0xFFFFFFFF {
			t := FileType(ft)
			h.FileType = &t
		}
	}
	if h.Architecture != "" {
		arch := h.Architecture
		count(arch+suffixContextLength, &h.ContextLength)
		count(arch+suffixBlockCount, &h.BlockCount)
		count(arch+suffixEmbeddingLength, &h.EmbeddingLength)
		count(arch+suffixKeyLength, &h.KeyLength)
		count(arch+suffixValueLength, &h.ValueLength)
		heads := func(key string, dst *[]uint64) {
			if v, ok := facts[key]; ok && err == nil {
				switch {
				case v.kind == typeArray && v.elem != typeBool:
					*dst = v.array
				case v.isInt && !v.neg:
					*dst = []uint64{v.num}
				default:
					err = malformed(errors.New(key + " is not a non-negative integer or integer array"))
				}
			}
		}
		heads(arch+suffixHeadCount, &h.HeadCount)
		heads(arch+suffixHeadCountKV, &h.HeadCountKV)
		count(arch+suffixSlidingWindow, &h.SlidingWindow)
		count(arch+suffixKeyLengthSWA, &h.KeyLengthSWA)
		count(arch+suffixValueLengthSWA, &h.ValueLengthSWA)
		if v, ok := facts[arch+suffixSlidingPattern]; ok && err == nil {
			switch {
			case v.kind == typeArray:
				h.SlidingWindowLayers = make([]bool, len(v.array))
				for i, x := range v.array {
					h.SlidingWindowLayers[i] = x != 0
				}
			case v.isInt && !v.neg:
				h.SlidingWindowPeriod = v.num
			default:
				err = malformed(errors.New(arch + suffixSlidingPattern + " is not a non-negative integer or a per-layer array"))
			}
		}
	}
	return err
}
