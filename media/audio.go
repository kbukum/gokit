package media

// detectAudio checks for known audio format signatures.
func detectAudio(data []byte) (Info, bool) {
	if len(data) < 4 {
		return Info{}, false
	}

	// WAV — RIFF....WAVE
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return Info{Type: Audio, Format: "wav", MimeType: "audio/wav", Container: "RIFF"}, true
	}

	// FLAC — "fLaC" magic.
	if data[0] == 'f' && data[1] == 'L' && data[2] == 'a' && data[3] == 'C' {
		return Info{Type: Audio, Format: "flac", MimeType: "audio/flac"}, true
	}

	// OGG — "OggS" magic.
	if data[0] == 'O' && data[1] == 'g' && data[2] == 'g' && data[3] == 'S' {
		return Info{Type: Audio, Format: "ogg", MimeType: "audio/ogg", Container: "Ogg"}, true
	}

	// AAC — ADTS frame sync: 0xFF 0xF0 (12-bit sync word, protection absent bit = 0).
	// Must check before MP3 since ADTS overlaps with MP3 frame sync.
	if data[0] == 0xFF && (data[1]&0xF0) == 0xF0 && (data[1]&0x06) == 0x00 {
		return Info{Type: Audio, Format: "aac", MimeType: "audio/aac"}, true
	}

	// MP3 — ID3 tag or frame sync.
	if data[0] == 'I' && data[1] == 'D' && data[2] == '3' {
		return Info{Type: Audio, Format: "mp3", MimeType: "audio/mpeg"}, true
	}
	// MP3 frame sync: 0xFF followed by byte with top 3 bits set (0xE0 mask).
	// Exclude AAC range (already caught above).
	if data[0] == 0xFF && (data[1]&0xE0) == 0xE0 {
		return Info{Type: Audio, Format: "mp3", MimeType: "audio/mpeg"}, true
	}

	// MIDI — "MThd" magic.
	if data[0] == 'M' && data[1] == 'T' && data[2] == 'h' && data[3] == 'd' {
		return Info{Type: Audio, Format: "midi", MimeType: "audio/midi"}, true
	}

	// AIFF — "FORM....AIFF"
	if len(data) >= 12 && string(data[0:4]) == "FORM" && string(data[8:12]) == "AIFF" {
		return Info{Type: Audio, Format: "aiff", MimeType: "audio/aiff", Container: "IFF"}, true
	}

	return Info{}, false
}
