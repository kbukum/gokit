package media

import "bytes"

var (
	jpegSignature          = []byte{0xFF, 0xD8, 0xFF}
	pngSignature           = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	gif87aSignature        = []byte("GIF87a")
	gif89aSignature        = []byte("GIF89a")
	bmpSignature           = []byte("BM")
	tiffLittleEndSignature = []byte{'I', 'I', 0x2A, 0x00}
	tiffBigEndSignature    = []byte{'M', 'M', 0x00, 0x2A}
	icoSignature           = []byte{0x00, 0x00, 0x01, 0x00}
)

// detectImage checks for known image format signatures.
func detectImage(data []byte) (Info, bool) {
	if len(data) < 4 {
		return Info{}, false
	}

	// JPEG — 0xFF 0xD8 0xFF
	if bytes.HasPrefix(data, jpegSignature) {
		return Info{Type: Image, Format: FormatJPEG, MimeType: "image/jpeg"}, true
	}

	// PNG — 8-byte signature: 0x89 PNG \r \n 0x1A \n
	if bytes.HasPrefix(data, pngSignature) {
		return Info{Type: Image, Format: FormatPNG, MimeType: "image/png"}, true
	}

	// GIF — "GIF87a" or "GIF89a"
	if len(data) >= len(gif87aSignature) &&
		(bytes.HasPrefix(data, gif87aSignature) || bytes.HasPrefix(data, gif89aSignature)) {
		return Info{Type: Image, Format: FormatGIF, MimeType: "image/gif"}, true
	}

	// WebP — RIFF....WEBP
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return Info{Type: Image, Format: FormatWebP, MimeType: "image/webp", Container: "RIFF"}, true
	}

	// BMP — "BM"
	if bytes.HasPrefix(data, bmpSignature) {
		return Info{Type: Image, Format: FormatBMP, MimeType: "image/bmp"}, true
	}

	// TIFF — "II" (little-endian) or "MM" (big-endian) + 0x002A
	if bytes.HasPrefix(data, tiffLittleEndSignature) ||
		bytes.HasPrefix(data, tiffBigEndSignature) {
		return Info{Type: Image, Format: FormatTIFF, MimeType: "image/tiff"}, true
	}

	// ICO — 0x00 0x00 0x01 0x00
	if bytes.HasPrefix(data, icoSignature) {
		return Info{Type: Image, Format: FormatICO, MimeType: "image/x-icon"}, true
	}

	// AVIF / HEIF — ftyp box with an image-specific brand (video ftyp handling
	// defers these to detectImage).
	if brand, ok := ftypBrand(data); ok {
		switch brand {
		case "avif", "avis":
			return Info{Type: Image, Format: FormatAVIF, MimeType: "image/avif", Container: "ISO BMFF"}, true
		case "heic", "heix", "heif":
			return Info{Type: Image, Format: FormatHEIF, MimeType: "image/heif", Container: "ISO BMFF"}, true
		}
	}

	return Info{}, false
}
