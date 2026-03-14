package media

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_JPEG(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	info := Detect(data)
	assertInfo(t, info, Image, "jpeg", "image/jpeg")
}

func TestDetect_PNG(t *testing.T) {
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	info := Detect(data)
	assertInfo(t, info, Image, "png", "image/png")
}

func TestDetect_GIF(t *testing.T) {
	data := []byte{'G', 'I', 'F', '8', '9', 'a'}
	info := Detect(data)
	assertInfo(t, info, Image, "gif", "image/gif")
}

func TestDetect_WebP(t *testing.T) {
	data := make([]byte, 12)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	info := Detect(data)
	assertInfo(t, info, Image, "webp", "image/webp")
}

func TestDetect_BMP(t *testing.T) {
	data := []byte{'B', 'M', 0x00, 0x00}
	info := Detect(data)
	assertInfo(t, info, Image, "bmp", "image/bmp")
}

func TestDetect_TIFF_LittleEndian(t *testing.T) {
	data := []byte{'I', 'I', 0x2A, 0x00}
	info := Detect(data)
	assertInfo(t, info, Image, "tiff", "image/tiff")
}

func TestDetect_TIFF_BigEndian(t *testing.T) {
	data := []byte{'M', 'M', 0x00, 0x2A}
	info := Detect(data)
	assertInfo(t, info, Image, "tiff", "image/tiff")
}

func TestDetect_AVIF(t *testing.T) {
	data := make([]byte, 12)
	// ftyp box at offset 4
	copy(data[4:8], "ftyp")
	copy(data[8:12], "avif")
	info := Detect(data)
	assertInfo(t, info, Image, "avif", "image/avif")
}

func TestDetect_HEIF(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "heic")
	info := Detect(data)
	assertInfo(t, info, Image, "heif", "image/heif")
}

func TestDetect_MP4(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "isom")
	info := Detect(data)
	assertInfo(t, info, Video, "mp4", "video/mp4")
}

func TestDetect_MOV(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "qt  ")
	info := Detect(data)
	assertInfo(t, info, Video, "mov", "video/quicktime")
}

func TestDetect_M4A(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "M4A ")
	info := Detect(data)
	assertInfo(t, info, Audio, "m4a", "audio/mp4")
}

func TestDetect_WebM(t *testing.T) {
	// EBML header + webm doctype
	data := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
		'w', 'e', 'b', 'm', 0x00, 0x00, 0x00, 0x00}
	info := Detect(data)
	assertInfo(t, info, Video, "webm", "video/webm")
}

func TestDetect_MKV(t *testing.T) {
	// EBML header without webm doctype
	data := []byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	info := Detect(data)
	assertInfo(t, info, Video, "mkv", "video/x-matroska")
}

func TestDetect_AVI(t *testing.T) {
	data := make([]byte, 12)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "AVI ")
	info := Detect(data)
	assertInfo(t, info, Video, "avi", "video/x-msvideo")
}

func TestDetect_FLV(t *testing.T) {
	data := []byte{'F', 'L', 'V', 0x01, 0x05}
	info := Detect(data)
	assertInfo(t, info, Video, "flv", "video/x-flv")
}

func TestDetect_WAV(t *testing.T) {
	data := make([]byte, 12)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WAVE")
	info := Detect(data)
	assertInfo(t, info, Audio, "wav", "audio/wav")
}

func TestDetect_FLAC(t *testing.T) {
	data := []byte{'f', 'L', 'a', 'C', 0x00}
	info := Detect(data)
	assertInfo(t, info, Audio, "flac", "audio/flac")
}

func TestDetect_OGG(t *testing.T) {
	data := []byte{'O', 'g', 'g', 'S', 0x00}
	info := Detect(data)
	assertInfo(t, info, Audio, "ogg", "audio/ogg")
}

func TestDetect_MP3_ID3(t *testing.T) {
	data := []byte{'I', 'D', '3', 0x04, 0x00}
	info := Detect(data)
	assertInfo(t, info, Audio, "mp3", "audio/mpeg")
}

func TestDetect_MP3_FrameSync(t *testing.T) {
	data := []byte{0xFF, 0xFB, 0x90, 0x00}
	info := Detect(data)
	assertInfo(t, info, Audio, "mp3", "audio/mpeg")
}

func TestDetect_MIDI(t *testing.T) {
	data := []byte{'M', 'T', 'h', 'd', 0x00, 0x00}
	info := Detect(data)
	assertInfo(t, info, Audio, "midi", "audio/midi")
}

func TestDetect_AIFF(t *testing.T) {
	data := make([]byte, 12)
	copy(data[0:4], "FORM")
	copy(data[8:12], "AIFF")
	info := Detect(data)
	assertInfo(t, info, Audio, "aiff", "audio/aiff")
}

func TestDetect_Text(t *testing.T) {
	data := []byte("Hello, world! This is a plain text file.\nWith multiple lines.\n")
	info := Detect(data)
	assertInfo(t, info, Text, "txt", "text/plain")
}

func TestDetect_TextUnicode(t *testing.T) {
	data := []byte("Héllo wörld! こんにちは 🌍\n")
	info := Detect(data)
	assertInfo(t, info, Text, "txt", "text/plain")
}

func TestDetect_ICO(t *testing.T) {
	data := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}
	info := Detect(data)
	assertInfo(t, info, Image, "ico", "image/x-icon")
}

func TestDetect_AAC(t *testing.T) {
	data := []byte{0xFF, 0xF1, 0x50, 0x80} // ADTS header
	info := Detect(data)
	assertInfo(t, info, Audio, "aac", "audio/aac")
}

func TestDetect_MPEGTS(t *testing.T) {
	data := make([]byte, 377)
	data[0] = 0x47
	data[188] = 0x47
	info := Detect(data)
	assertInfo(t, info, Video, "ts", "video/mp2t")
}

func TestDetect_M4V(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "M4V ")
	info := Detect(data)
	assertInfo(t, info, Video, "m4v", "video/x-m4v")
}

func TestDetect_ShortData(t *testing.T) {
	// 1 byte — too short for most detectors but shouldn't panic.
	info := Detect([]byte{0x00})
	if info.Type != Unknown {
		t.Errorf("expected Unknown for 1-byte input, got %v", info.Type)
	}

	// 3 bytes.
	info = Detect([]byte{0x00, 0x01, 0x02})
	if info.Type != Unknown {
		t.Errorf("expected Unknown for 3-byte input, got %v", info.Type)
	}
}

func TestDetect_TextWithControlChars(t *testing.T) {
	// Mostly printable but has some control chars — should fail text detection.
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i) // bytes 0-99, many control chars
	}
	info := Detect(data)
	if info.Type == Text {
		t.Error("data with many control chars should not be detected as text")
	}
}

func TestDetect_GenericFtyp(t *testing.T) {
	// Unknown ftyp brand defaults to MP4.
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "xxxx")
	info := Detect(data)
	assertInfo(t, info, Video, "mp4", "video/mp4")
}

func TestDetect_M4B(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "M4B ")
	info := Detect(data)
	assertInfo(t, info, Audio, "m4a", "audio/mp4")
}

func TestDetect_AVIS(t *testing.T) {
	data := make([]byte, 12)
	copy(data[4:8], "ftyp")
	copy(data[8:12], "avis")
	info := Detect(data)
	assertInfo(t, info, Image, "avif", "image/avif")
}

func TestDetect_Empty(t *testing.T) {
	info := Detect(nil)
	if info.Type != Unknown {
		t.Errorf("expected Unknown for empty input, got %v", info.Type)
	}

	info = Detect([]byte{})
	if info.Type != Unknown {
		t.Errorf("expected Unknown for empty slice, got %v", info.Type)
	}
}

func TestDetect_BinaryGarbage(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x80, 0x81, 0x82, 0xFE, 0xFF}
	info := Detect(data)
	if info.Type != Unknown {
		t.Errorf("expected Unknown for binary garbage, got %v", info.Type)
	}
}

func TestDetectReader(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	info, err := DetectReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInfo(t, info, Image, "jpeg", "image/jpeg")
}

func TestDetectReader_Empty(t *testing.T) {
	_, err := DetectReader(bytes.NewReader(nil))
	if err == nil {
		t.Error("expected error for empty reader")
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		t    Type
		want string
	}{
		{Unknown, "unknown"},
		{Video, "video"},
		{Audio, "audio"},
		{Image, "image"},
		{Text, "text"},
	}
	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("Type(%d).String() = %q, want %q", tt.t, got, tt.want)
		}
	}
}

func TestInfoJSON(t *testing.T) {
	info := Info{Type: Video, Format: "mp4", MimeType: "video/mp4", Container: "ISO BMFF"}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Info
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != info {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, info)
	}
}

// TestDetectFile_WithTempFile creates a temp file and verifies detection.
func TestDetectFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	// Write PNG header.
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if err := os.WriteFile(path, pngHeader, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := DetectFile(path)
	if err != nil {
		t.Fatalf("DetectFile: %v", err)
	}
	assertInfo(t, info, Image, "png", "image/png")
}

func TestDetectFile_NotFound(t *testing.T) {
	_, err := DetectFile("/nonexistent/path/file.bin")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func assertInfo(t *testing.T, got Info, wantType Type, wantFormat, wantMime string) {
	t.Helper()
	if got.Type != wantType {
		t.Errorf("Type = %v, want %v", got.Type, wantType)
	}
	if got.Format != wantFormat {
		t.Errorf("Format = %q, want %q", got.Format, wantFormat)
	}
	if got.MimeType != wantMime {
		t.Errorf("MimeType = %q, want %q", got.MimeType, wantMime)
	}
}
