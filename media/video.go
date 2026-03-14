package media

// detectVideo checks for known video format signatures.
func detectVideo(data []byte) (Info, bool) {
	if len(data) < 4 {
		return Info{}, false
	}

	// MP4 / MOV / M4V — ftyp box at offset 4.
	if len(data) >= 12 && data[4] == 'f' && data[5] == 't' && data[6] == 'y' && data[7] == 'p' {
		brand := string(data[8:12])

		// Image formats that use ftyp boxes — return early so detectImage isn't needed.
		switch brand {
		case "avif", "avis":
			return Info{}, false // Let detectImage handle it.
		case "heic", "heix", "heif":
			return Info{}, false // Let detectImage handle it.
		case "isom", "iso2", "mp41", "mp42", "avc1", "dash":
			return Info{Type: Video, Format: "mp4", MimeType: "video/mp4", Container: "ISO BMFF"}, true
		case "qt  ":
			return Info{Type: Video, Format: "mov", MimeType: "video/quicktime", Container: "QuickTime"}, true
		case "M4V ", "M4VH", "M4VP":
			return Info{Type: Video, Format: "m4v", MimeType: "video/x-m4v", Container: "ISO BMFF"}, true
		case "M4A ", "M4B ":
			return Info{Type: Audio, Format: "m4a", MimeType: "audio/mp4", Container: "ISO BMFF"}, true
		default:
			// Generic ftyp — assume video MP4.
			return Info{Type: Video, Format: "mp4", MimeType: "video/mp4", Container: "ISO BMFF"}, true
		}
	}

	// WebM / MKV — EBML header: 0x1A 0x45 0xDF 0xA3
	if data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		// Search for DocType in the first bytes.
		docType := findEBMLDocType(data)
		switch docType {
		case "webm":
			return Info{Type: Video, Format: "webm", MimeType: "video/webm", Container: "Matroska"}, true
		default:
			return Info{Type: Video, Format: "mkv", MimeType: "video/x-matroska", Container: "Matroska"}, true
		}
	}

	// AVI — RIFF....AVI
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "AVI " {
		return Info{Type: Video, Format: "avi", MimeType: "video/x-msvideo", Container: "RIFF"}, true
	}

	// FLV — "FLV" header.
	if data[0] == 'F' && data[1] == 'L' && data[2] == 'V' {
		return Info{Type: Video, Format: "flv", MimeType: "video/x-flv", Container: "FLV"}, true
	}

	// MPEG-TS — sync byte 0x47 every 188 bytes.
	if data[0] == 0x47 && len(data) >= 376 && data[188] == 0x47 {
		return Info{Type: Video, Format: "ts", MimeType: "video/mp2t", Container: "MPEG-TS"}, true
	}

	return Info{}, false
}

// findEBMLDocType searches for "webm" or "matroska" in the EBML header.
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
