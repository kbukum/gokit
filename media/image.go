package media

import "bytes"

// detectImage checks for known image format signatures.
func detectImage(data []byte) (Info, bool) {
	if len(data) < 4 {
		return Info{}, false
	}

	// JPEG — 0xFF 0xD8 0xFF
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return Info{Type: Image, Format: "jpeg", MimeType: "image/jpeg"}, true
	}

	// PNG — 8-byte signature: 0x89 PNG \r \n 0x1A \n
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}) {
		return Info{Type: Image, Format: "png", MimeType: "image/png"}, true
	}

	// GIF — "GIF87a" or "GIF89a"
	if bytes.HasPrefix(data, []byte("GIF")) {
		return Info{Type: Image, Format: "gif", MimeType: "image/gif"}, true
	}

	// WebP — RIFF....WEBP
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return Info{Type: Image, Format: "webp", MimeType: "image/webp", Container: "RIFF"}, true
	}

	// BMP — "BM"
	if bytes.HasPrefix(data, []byte("BM")) {
		return Info{Type: Image, Format: "bmp", MimeType: "image/bmp"}, true
	}

	// TIFF — "II" (little-endian) or "MM" (big-endian) + 0x002A
	if bytes.HasPrefix(data, []byte{'I', 'I', 0x2A, 0x00}) ||
		bytes.HasPrefix(data, []byte{'M', 'M', 0x00, 0x2A}) {
		return Info{Type: Image, Format: "tiff", MimeType: "image/tiff"}, true
	}

	// ICO — 0x00 0x00 0x01 0x00
	if bytes.HasPrefix(data, []byte{0x00, 0x00, 0x01, 0x00}) {
		return Info{Type: Image, Format: "ico", MimeType: "image/x-icon"}, true
	}

	// AVIF / HEIF — ftyp box with avif/heif/heic brand (already handled as video ftyp,
	// but if brand is image-specific, override here).
	if len(data) >= 12 && data[4] == 'f' && data[5] == 't' && data[6] == 'y' && data[7] == 'p' {
		brand := string(data[8:12])
		switch brand {
		case "avif", "avis":
			return Info{Type: Image, Format: "avif", MimeType: "image/avif", Container: "ISO BMFF"}, true
		case "heic", "heix", "heif":
			return Info{Type: Image, Format: "heif", MimeType: "image/heif", Container: "ISO BMFF"}, true
		}
	}

	return Info{}, false
}
