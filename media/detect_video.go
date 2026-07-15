package media

import "bytes"

var (
	ebmlSignature = []byte{0x1A, 0x45, 0xDF, 0xA3}
	flvSignature  = []byte("FLV")
)

// detectVideo checks for known video format signatures.
func detectVideo(data []byte) (Info, bool) {
	if len(data) < 4 {
		return Info{}, false
	}

	// MP4 / MOV / M4V — ftyp box at offset 4.
	if brand, ok := ftypBrand(data); ok {
		switch brand {
		case "avif", "avis", "heic", "heix", "heif":
			return Info{}, false // Image brands — let detectImage handle it.
		case "isom", "iso2", "mp41", "mp42", "avc1", "dash":
			return Info{Type: Video, Format: FormatMP4, MimeType: "video/mp4", Container: "ISO BMFF"}, true
		case "qt  ":
			return Info{Type: Video, Format: FormatMOV, MimeType: "video/quicktime", Container: "QuickTime"}, true
		case "M4V ", "M4VH", "M4VP":
			return Info{Type: Video, Format: FormatM4V, MimeType: "video/x-m4v", Container: "ISO BMFF"}, true
		case "M4A ", "M4B ":
			return Info{Type: Audio, Format: FormatM4A, MimeType: "audio/mp4", Container: "ISO BMFF"}, true
		default:
			// Generic ftyp — assume video MP4.
			return Info{Type: Video, Format: FormatMP4, MimeType: "video/mp4", Container: "ISO BMFF"}, true
		}
	}

	// WebM / MKV — EBML header: 0x1A 0x45 0xDF 0xA3
	if bytes.HasPrefix(data, ebmlSignature) {
		if findEBMLDocType(data) == "webm" {
			return Info{Type: Video, Format: FormatWebM, MimeType: "video/webm", Container: "Matroska"}, true
		}
		return Info{Type: Video, Format: FormatMKV, MimeType: "video/x-matroska", Container: "Matroska"}, true
	}

	// AVI — RIFF....AVI
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "AVI " {
		return Info{Type: Video, Format: FormatAVI, MimeType: "video/x-msvideo", Container: "RIFF"}, true
	}

	// FLV — "FLV" header.
	if bytes.HasPrefix(data, flvSignature) {
		return Info{Type: Video, Format: FormatFLV, MimeType: "video/x-flv", Container: "FLV"}, true
	}

	// MPEG-TS — sync byte 0x47 every 188 bytes.
	if data[0] == 0x47 && len(data) >= 376 && data[188] == 0x47 {
		return Info{Type: Video, Format: FormatTS, MimeType: "video/mp2t", Container: "MPEG-TS"}, true
	}

	return Info{}, false
}

// findEBMLDocType searches the EBML header for the "webm" DocType marker.
func findEBMLDocType(data []byte) string {
	limit := len(data)
	if limit > 64 {
		limit = 64
	}
	for i := 0; i < limit-4; i++ {
		if string(data[i:i+4]) == "webm" {
			return "webm"
		}
	}
	return "matroska"
}
