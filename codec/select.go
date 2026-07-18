package codec

import (
	"path/filepath"
	"strings"
)

// CodecForName returns a codec for a format name (for example "toml", "json"), matched case-insensitively. The boolean is false when no codec matches.
func CodecForName(name string) (Codec, bool) {
	switch strings.ToLower(name) {
	case "toml":
		return NewTOMLCodec(), true
	case "json":
		return PrettyJSON(), true
	default:
		return nil, false
	}
}

// CodecForPath returns a codec for path's file extension. The boolean is false when the extension is missing or unrecognized.
func CodecForPath(path string) (Codec, bool) {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if ext == "" {
		return nil, false
	}
	return CodecForName(ext)
}
