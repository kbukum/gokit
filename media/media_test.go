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

func TestDetect_GIFRejectsTruncatedOrInvalidHeaders(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("GIF"),
		[]byte("GIF8"),
		[]byte("GIF89"),
		[]byte("GIF87"),
		[]byte("GIF00a"),
	} {
		info := Detect(data)
		if info.Type == Image || info.Format == "gif" {
			t.Fatalf("expected non-GIF for %q, got %#v", data, info)
		}
	}
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
	data := []byte{
		0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
		'w', 'e', 'b', 'm', 0x00, 0x00, 0x00, 0x00,
	}
	info := Detect(data)
	assertInfo(t, info, Video, "webm", "video/webm")
}

func TestDetect_MKV(t *testing.T) {
	// EBML header without webm doctype
	data := []byte{
		0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
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

	// 4 bytes exercise the shortest guarded image/video signatures without
	// allowing longer detectors to read past the slice.
	for _, tc := range []struct {
		name   string
		data   []byte
		typ    Type
		format string
	}{
		{name: "jpeg", data: []byte{0xFF, 0xD8, 0xFF, 0x00}, typ: Image, format: "jpeg"},
		{name: "flv", data: []byte{'F', 'L', 'V', 0x00}, typ: Video, format: "flv"},
		{name: "ebml", data: []byte{0x1A, 0x45, 0xDF, 0xA3}, typ: Video, format: "mkv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := Detect(tc.data)
			if info.Type != tc.typ || info.Format != tc.format {
				t.Fatalf("expected %s/%s, got %#v", tc.typ, tc.format, info)
			}
		})
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

// ---------------------------------------------------------------------------
// Security tests
// ---------------------------------------------------------------------------

func TestDetect_ScriptEmbeddedInJPEG(t *testing.T) {
	data := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'},
		[]byte(`<script>alert(1)</script>`)...)
	info := Detect(data)
	assertInfo(t, info, Image, "jpeg", "image/jpeg")
}

func TestDetect_PHPInsideGIF(t *testing.T) {
	data := append([]byte{'G', 'I', 'F', '8', '9', 'a'},
		[]byte(`<?php echo "hacked"; ?>`)...)
	info := Detect(data)
	assertInfo(t, info, Image, "gif", "image/gif")
}

func TestDetect_PolyglotPDFJPEG(t *testing.T) {
	header := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10} //nolint:prealloc // test fixture; readability over micro-perf
	padding := make([]byte, 100)
	pdfSig := []byte("%PDF-1.4")
	data := append(append(header, padding...), pdfSig...) //nolint:gocritic // intentional: creating new slice for test data
	info := Detect(data)
	assertInfo(t, info, Image, "jpeg", "image/jpeg")
}

func TestDetect_NullBytePadding(t *testing.T) {
	data := make([]byte, 1000) // all zeros
	info := Detect(data)
	if info.Type != Unknown {
		t.Errorf("expected Unknown for null-padded data, got %v", info.Type)
	}
}

func TestDetect_ScriptInsidePNG(t *testing.T) {
	header := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}      //nolint:prealloc // test fixture; readability over micro-perf
	data := append(header, []byte(`<script>alert("xss")</script>`)...) //nolint:gocritic // intentional: creating new slice for test data
	info := Detect(data)
	assertInfo(t, info, Image, "png", "image/png")
}

// ---------------------------------------------------------------------------
// Edge-case tests
// ---------------------------------------------------------------------------

func TestDetect_ExactlyFourBytes(t *testing.T) {
	data := []byte{0x12, 0x34, 0x56, 0x78}
	info := Detect(data)
	if info.Type != Unknown {
		t.Errorf("expected Unknown for 4 random bytes, got %v", info.Type)
	}
}

func TestDetect_RepeatedMagicBytes(t *testing.T) {
	// JPEG header followed by PNG header — first detector (JPEG) wins.
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10} //nolint:prealloc // test fixture; readability over micro-perf
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	data := append(jpeg, png...) //nolint:gocritic // intentional: creating new slice for test data
	info := Detect(data)
	assertInfo(t, info, Image, "jpeg", "image/jpeg")
}

func TestDetect_MaxDetectBytesLimit(t *testing.T) {
	// First 4096 bytes are printable text; bytes beyond are control chars.
	// isText only checks the first 4096 bytes, so it should still be text.
	text := make([]byte, 4096) //nolint:prealloc // test fixture; readability over micro-perf
	for i := range text {
		text[i] = 'A'
	}
	garbage := make([]byte, 1000)
	for i := range garbage {
		garbage[i] = 0x01
	}
	data := append(text, garbage...) //nolint:gocritic // intentional: creating new slice for test data
	info := Detect(data)
	assertInfo(t, info, Text, "txt", "text/plain")
}

func TestDetect_TextBoundary95Percent(t *testing.T) {
	t.Run("exactly_95_percent", func(t *testing.T) {
		// 100 bytes: 95 printable, 5 non-printable control chars.
		data := make([]byte, 100)
		for i := 0; i < 95; i++ {
			data[i] = 'A'
		}
		for i := 95; i < 100; i++ {
			data[i] = 0x01 // non-printable control char
		}
		info := Detect(data)
		assertInfo(t, info, Text, "txt", "text/plain")
	})

	t.Run("below_95_percent", func(t *testing.T) {
		// 100 bytes: 94 printable, 6 non-printable control chars.
		data := make([]byte, 100)
		for i := 0; i < 94; i++ {
			data[i] = 'A'
		}
		for i := 94; i < 100; i++ {
			data[i] = 0x01
		}
		info := Detect(data)
		if info.Type != Unknown {
			t.Errorf("expected Unknown for <95%% printable, got %v", info.Type)
		}
	})
}

func TestDetect_LargeReaderData(t *testing.T) {
	// Reader with >4096 bytes of text. DetectReader reads at most 4096.
	data := make([]byte, 8192)
	for i := range data {
		data[i] = 'Z'
	}
	info, err := DetectReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInfo(t, info, Text, "txt", "text/plain")
}

func TestDetect_GIF87a(t *testing.T) {
	data := []byte{'G', 'I', 'F', '8', '7', 'a'}
	info := Detect(data)
	assertInfo(t, info, Image, "gif", "image/gif")
}

func TestDetect_DetectFileEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := DetectFile(path)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestDetect_AllWhitespace(t *testing.T) {
	data := []byte("   \t\t\n\n\r\n   \t  \n")
	info := Detect(data)
	assertInfo(t, info, Text, "txt", "text/plain")
}

// ---------------------------------------------------------------------------
// Additional format tests
// ---------------------------------------------------------------------------

func TestDetect_MP4_AllBrands(t *testing.T) {
	brands := []string{"iso2", "mp41", "mp42", "avc1", "dash"}
	for _, brand := range brands {
		t.Run(brand, func(t *testing.T) {
			data := make([]byte, 12)
			copy(data[4:8], "ftyp")
			copy(data[8:12], brand)
			info := Detect(data)
			assertInfo(t, info, Video, "mp4", "video/mp4")
		})
	}
}

func TestDetect_M4V_AllBrands(t *testing.T) {
	brands := []string{"M4VH", "M4VP"}
	for _, brand := range brands {
		t.Run(brand, func(t *testing.T) {
			data := make([]byte, 12)
			copy(data[4:8], "ftyp")
			copy(data[8:12], brand)
			info := Detect(data)
			assertInfo(t, info, Video, "m4v", "video/x-m4v")
		})
	}
}

func TestDetect_HEIF_AllBrands(t *testing.T) {
	brands := []string{"heix", "heif"}
	for _, brand := range brands {
		t.Run(brand, func(t *testing.T) {
			data := make([]byte, 12)
			copy(data[4:8], "ftyp")
			copy(data[8:12], brand)
			info := Detect(data)
			assertInfo(t, info, Image, "heif", "image/heif")
		})
	}
}

func TestDetect_MultipleMP3SyncPatterns(t *testing.T) {
	patterns := []struct {
		name  string
		byte1 byte
	}{
		{"0xE3", 0xE3},
		{"0xEB", 0xEB},
	}
	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			data := []byte{0xFF, p.byte1, 0x90, 0x00}
			info := Detect(data)
			assertInfo(t, info, Audio, "mp3", "audio/mpeg")
		})
	}
}

// ---------------------------------------------------------------------------
// Container field validation
// ---------------------------------------------------------------------------

func TestDetect_ContainerField(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		container string
	}{
		{"AVI_RIFF", func() []byte {
			d := make([]byte, 12)
			copy(d[0:4], "RIFF")
			copy(d[8:12], "AVI ")
			return d
		}(), "RIFF"},
		{"WAV_RIFF", func() []byte {
			d := make([]byte, 12)
			copy(d[0:4], "RIFF")
			copy(d[8:12], "WAVE")
			return d
		}(), "RIFF"},
		{"WebP_RIFF", func() []byte {
			d := make([]byte, 12)
			copy(d[0:4], "RIFF")
			copy(d[8:12], "WEBP")
			return d
		}(), "RIFF"},
		{"MP4_ISO_BMFF", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "isom")
			return d
		}(), "ISO BMFF"},
		{"M4V_ISO_BMFF", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "M4V ")
			return d
		}(), "ISO BMFF"},
		{"M4A_ISO_BMFF", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "M4A ")
			return d
		}(), "ISO BMFF"},
		{"AVIF_ISO_BMFF", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "avif")
			return d
		}(), "ISO BMFF"},
		{"HEIF_ISO_BMFF", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "heic")
			return d
		}(), "ISO BMFF"},
		{"MOV_QuickTime", func() []byte {
			d := make([]byte, 12)
			copy(d[4:8], "ftyp")
			copy(d[8:12], "qt  ")
			return d
		}(), "QuickTime"},
		{"WebM_Matroska", []byte{
			0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
			'w', 'e', 'b', 'm', 0x00, 0x00, 0x00, 0x00,
		}, "Matroska"},
		{"MKV_Matroska", []byte{
			0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}, "Matroska"},
		{"FLV_Container", []byte{'F', 'L', 'V', 0x01, 0x05}, "FLV"},
		{"MPEGTS_Container", func() []byte {
			d := make([]byte, 377)
			d[0] = 0x47
			d[188] = 0x47
			return d
		}(), "MPEG-TS"},
		{"OGG_Container", []byte{'O', 'g', 'g', 'S', 0x00}, "Ogg"},
		{"AIFF_IFF", func() []byte {
			d := make([]byte, 12)
			copy(d[0:4], "FORM")
			copy(d[8:12], "AIFF")
			return d
		}(), "IFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Detect(tt.data)
			if info.Container != tt.container {
				t.Errorf("Container = %q, want %q", info.Container, tt.container)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip tests
// ---------------------------------------------------------------------------

func TestInfoJSON_AllTypes(t *testing.T) {
	types := []struct {
		tp   Type
		name string
	}{
		{Unknown, "unknown"},
		{Video, "video"},
		{Audio, "audio"},
		{Image, "image"},
		{Text, "text"},
	}
	for _, tt := range types {
		t.Run(tt.name, func(t *testing.T) {
			original := Info{
				Type:      tt.tp,
				Format:    "fmt",
				MimeType:  "type/fmt",
				Container: "Box",
			}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded Info
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded != original {
				t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
			}
		})
	}
}

func TestInfoJSON_EmptyInfo(t *testing.T) {
	original := Info{}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Info
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

// ---------------------------------------------------------------------------
// Type edge tests
// ---------------------------------------------------------------------------

func TestTypeString_OutOfRange(t *testing.T) {
	if got := Type(99).String(); got != "unknown" {
		t.Errorf("Type(99).String() = %q, want %q", got, "unknown")
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
