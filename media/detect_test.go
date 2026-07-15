package media

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"
)

// ftyp builds a 12-byte ISO BMFF header with the given brand at offset 8.
func ftyp(brand string) []byte {
	d := make([]byte, 12)
	copy(d[4:8], "ftyp")
	copy(d[8:12], brand)
	return d
}

// riff builds a 12-byte RIFF header with the given form type at offset 8.
func riff(form string) []byte {
	d := make([]byte, 12)
	copy(d[0:4], "RIFF")
	copy(d[8:12], form)
	return d
}

// ebml builds a 16-byte Matroska/EBML header with an optional doctype at
// offset 8 (empty doctype yields a bare EBML stream, i.e. MKV).
func ebml(doctype string) []byte {
	d := make([]byte, 16)
	copy(d[0:4], []byte{0x1A, 0x45, 0xDF, 0xA3})
	copy(d[8:], doctype)
	return d
}

// mpegTS builds a minimal MPEG-TS stream with sync bytes 188 bytes apart.
func mpegTS() []byte {
	d := make([]byte, 377)
	d[0] = 0x47
	d[188] = 0x47
	return d
}

// TestFindEBMLDocType_MatchesLastWindow guards against an off-by-one that
// skipped the final 4-byte scan window, which would miss a "webm" doctype
// sitting at the very end of the inspected bytes.
func TestFindEBMLDocType_MatchesLastWindow(t *testing.T) {
	t.Parallel()
	data := append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, []byte("webm")...)
	if got := findEBMLDocType(data); got != "webm" {
		t.Errorf("findEBMLDocType = %q, want webm", got)
	}
}

// TestDetect_Signatures verifies that each recognized byte signature maps to the
// full expected [Info] (type, format, mime, and container) in one table. The
// 4-byte rows also prove the shortest guarded signatures never read past the
// slice.
func TestDetect_Signatures(t *testing.T) {
	t.Parallel()
	img := func(f Format, mime, container string) Info {
		return Info{Type: Image, Format: f, MimeType: mime, Container: container}
	}
	vid := func(f Format, mime, container string) Info {
		return Info{Type: Video, Format: f, MimeType: mime, Container: container}
	}
	aud := func(f Format, mime, container string) Info {
		return Info{Type: Audio, Format: f, MimeType: mime, Container: container}
	}
	txt := Info{Type: Text, Format: FormatText, MimeType: "text/plain"}
	tests := []struct {
		name string
		data []byte
		want Info
	}{
		// Images.
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, img(FormatJPEG, "image/jpeg", "")},
		{"jpeg_4byte", []byte{0xFF, 0xD8, 0xFF, 0x00}, img(FormatJPEG, "image/jpeg", "")},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, img(FormatPNG, "image/png", "")},
		{"gif89a", []byte("GIF89a"), img(FormatGIF, "image/gif", "")},
		{"gif87a", []byte("GIF87a"), img(FormatGIF, "image/gif", "")},
		{"webp", riff("WEBP"), img(FormatWebP, "image/webp", "RIFF")},
		{"bmp", []byte{'B', 'M', 0x00, 0x00}, img(FormatBMP, "image/bmp", "")},
		{"tiff_le", []byte{'I', 'I', 0x2A, 0x00}, img(FormatTIFF, "image/tiff", "")},
		{"tiff_be", []byte{'M', 'M', 0x00, 0x2A}, img(FormatTIFF, "image/tiff", "")},
		{"ico", []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, img(FormatICO, "image/x-icon", "")},
		{"avif", ftyp("avif"), img(FormatAVIF, "image/avif", "ISO BMFF")},
		{"avis", ftyp("avis"), img(FormatAVIF, "image/avif", "ISO BMFF")},
		{"heif_heic", ftyp("heic"), img(FormatHEIF, "image/heif", "ISO BMFF")},
		{"heif_heix", ftyp("heix"), img(FormatHEIF, "image/heif", "ISO BMFF")},
		{"heif_heif", ftyp("heif"), img(FormatHEIF, "image/heif", "ISO BMFF")},

		// Video.
		{"mp4_isom", ftyp("isom"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_iso2", ftyp("iso2"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_mp41", ftyp("mp41"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_mp42", ftyp("mp42"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_avc1", ftyp("avc1"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_dash", ftyp("dash"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mp4_unknown_brand", ftyp("xxxx"), vid(FormatMP4, "video/mp4", "ISO BMFF")},
		{"mov", ftyp("qt  "), vid(FormatMOV, "video/quicktime", "QuickTime")},
		{"m4v", ftyp("M4V "), vid(FormatM4V, "video/x-m4v", "ISO BMFF")},
		{"m4v_h", ftyp("M4VH"), vid(FormatM4V, "video/x-m4v", "ISO BMFF")},
		{"m4v_p", ftyp("M4VP"), vid(FormatM4V, "video/x-m4v", "ISO BMFF")},
		{"webm", ebml("webm"), vid(FormatWebM, "video/webm", "Matroska")},
		{"mkv", ebml(""), vid(FormatMKV, "video/x-matroska", "Matroska")},
		{"ebml_4byte", []byte{0x1A, 0x45, 0xDF, 0xA3}, vid(FormatMKV, "video/x-matroska", "Matroska")},
		{"avi", riff("AVI "), vid(FormatAVI, "video/x-msvideo", "RIFF")},
		{"flv", []byte{'F', 'L', 'V', 0x01, 0x05}, vid(FormatFLV, "video/x-flv", "FLV")},
		{"flv_4byte", []byte{'F', 'L', 'V', 0x00}, vid(FormatFLV, "video/x-flv", "FLV")},
		{"mpegts", mpegTS(), vid(FormatTS, "video/mp2t", "MPEG-TS")},

		// Audio.
		{"m4a", ftyp("M4A "), aud(FormatM4A, "audio/mp4", "ISO BMFF")},
		{"m4b", ftyp("M4B "), aud(FormatM4A, "audio/mp4", "ISO BMFF")},
		{"wav", riff("WAVE"), aud(FormatWAV, "audio/wav", "RIFF")},
		{"flac", []byte{'f', 'L', 'a', 'C', 0x00}, aud(FormatFLAC, "audio/flac", "")},
		{"ogg", []byte{'O', 'g', 'g', 'S', 0x00}, aud(FormatOGG, "audio/ogg", "Ogg")},
		{"aac", []byte{0xFF, 0xF1, 0x50, 0x80}, aud(FormatAAC, "audio/aac", "")},
		{"mp3_id3", []byte{'I', 'D', '3', 0x04, 0x00}, aud(FormatMP3, "audio/mpeg", "")},
		{"mp3_sync_fb", []byte{0xFF, 0xFB, 0x90, 0x00}, aud(FormatMP3, "audio/mpeg", "")},
		{"mp3_sync_e3", []byte{0xFF, 0xE3, 0x90, 0x00}, aud(FormatMP3, "audio/mpeg", "")},
		{"mp3_sync_eb", []byte{0xFF, 0xEB, 0x90, 0x00}, aud(FormatMP3, "audio/mpeg", "")},
		{"midi", []byte{'M', 'T', 'h', 'd', 0x00, 0x00}, aud(FormatMIDI, "audio/midi", "")},

		// Text.
		{"text_ascii", []byte("Hello, world! This is plain text.\nWith lines.\n"), txt},
		{"text_unicode", []byte("Héllo wörld! こんにちは 🌍\n"), txt},
		{"text_whitespace", []byte("   \t\t\n\n\r\n   \t  \n"), txt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Detect(tt.data); got != tt.want {
				t.Errorf("Detect() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestDetect_AIFF verifies AIFF detection including its IFF container, kept
// separate because FORM/AIFF uses the same RIFF-style helper as WAV but a
// different container family.
func TestDetect_AIFF(t *testing.T) {
	t.Parallel()
	d := make([]byte, 12)
	copy(d[0:4], "FORM")
	copy(d[8:12], "AIFF")
	want := Info{Type: Audio, Format: FormatAIFF, MimeType: "audio/aiff", Container: "IFF"}
	if got := Detect(d); got != want {
		t.Errorf("Detect(AIFF) = %#v, want %#v", got, want)
	}
}

// TestDetect_Unknown covers inputs that must not classify as any media type:
// too-short, empty, binary garbage, and truncated/invalid GIF headers.
func TestDetect_Unknown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"one_byte", []byte{0x00}},
		{"three_bytes", []byte{0x00, 0x01, 0x02}},
		{"four_random", []byte{0x12, 0x34, 0x56, 0x78}},
		{"binary_garbage", []byte{0x00, 0x01, 0x02, 0x03, 0x80, 0x81, 0x82, 0xFE, 0xFF}},
		{"null_padding", make([]byte, 1000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Detect(tt.data); got.Type != Unknown {
				t.Errorf("Detect(%s) = %#v, want Unknown", tt.name, got)
			}
		})
	}
}

// TestDetect_GIFRejectsTruncatedHeaders ensures partial or malformed GIF magic
// is never misclassified as a GIF image (these ASCII prefixes may still fall
// through to the text heuristic, which is acceptable).
func TestDetect_GIFRejectsTruncatedHeaders(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		[]byte("GIF"),
		[]byte("GIF8"),
		[]byte("GIF89"),
		[]byte("GIF87"),
		[]byte("GIF00a"),
	} {
		if got := Detect(data); got.Type == Image || got.Format == FormatGIF {
			t.Errorf("Detect(%q) = %#v, want non-GIF", data, got)
		}
	}
}

// TestDetect_TextHeuristic covers the UTF-8 printable-ratio classifier: control
// chars fail, the 95%% ratio boundary is exact, and only the first
// maxDetectBytes are sampled.
func TestDetect_TextHeuristic(t *testing.T) {
	t.Parallel()

	buildRatio := func(printable int) []byte {
		data := make([]byte, 100)
		for i := range data {
			if i < printable {
				data[i] = 'A'
			} else {
				data[i] = 0x01
			}
		}
		return data
	}

	t.Run("control_chars_reject", func(t *testing.T) {
		t.Parallel()
		data := make([]byte, 100)
		for i := range data {
			data[i] = byte(i) // many control chars
		}
		if got := Detect(data); got.Type == Text {
			t.Errorf("data with control chars classified as Text: %#v", got)
		}
	})

	t.Run("exactly_95_percent", func(t *testing.T) {
		t.Parallel()
		if got := Detect(buildRatio(95)); got.Type != Text {
			t.Errorf("95%% printable = %#v, want Text", got)
		}
	})

	t.Run("below_95_percent", func(t *testing.T) {
		t.Parallel()
		if got := Detect(buildRatio(94)); got.Type != Unknown {
			t.Errorf("94%% printable = %#v, want Unknown", got)
		}
	})

	t.Run("samples_only_first_window", func(t *testing.T) {
		t.Parallel()
		// First maxDetectBytes are text; trailing control bytes are past the
		// sampling window, so the input still classifies as text.
		data := concat(bytes.Repeat([]byte{'A'}, maxDetectBytes), bytes.Repeat([]byte{0x01}, 1000))
		if got := Detect(data); got.Type != Text {
			t.Errorf("windowed text = %#v, want Text", got)
		}
	})
}

// TestDetect_LeadingSignatureWins proves detection keys only on the leading
// bytes: appended payloads (polyglots, embedded scripts, a second signature)
// never change the classification.
func TestDetect_LeadingSignatureWins(t *testing.T) {
	t.Parallel()
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	gif := []byte("GIF89a")
	tests := []struct {
		name    string
		data    []byte
		wantFmt Format
	}{
		{"script_in_jpeg", concat(jpeg, []byte(`<script>alert(1)</script>`)), FormatJPEG},
		{"pdf_polyglot_after_jpeg", concat(jpeg, make([]byte, 100), []byte("%PDF-1.4")), FormatJPEG},
		{"script_in_png", concat(png, []byte(`<script>alert("xss")</script>`)), FormatPNG},
		{"php_in_gif", concat(gif, []byte(`<?php echo "hacked"; ?>`)), FormatGIF},
		{"png_after_jpeg", concat(jpeg, png), FormatJPEG},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Detect(tt.data); got.Format != tt.wantFmt {
				t.Errorf("Detect() format = %q, want %q", got.Format, tt.wantFmt)
			}
		})
	}
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestDetectReader(t *testing.T) {
	t.Parallel()
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	info, err := DetectReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DetectReader: %v", err)
	}
	assertInfo(t, info, Image, FormatJPEG, "image/jpeg")
}

func TestDetectReader_Empty(t *testing.T) {
	t.Parallel()
	if _, err := DetectReader(bytes.NewReader(nil)); err == nil {
		t.Error("expected error for empty reader")
	}
}

func TestDetectReader_ChunkedReadFillsWindow(t *testing.T) {
	t.Parallel()
	// A reader that yields one byte per Read must not truncate detection: the
	// 12-byte RIFF/WEBP signature is only decidable once bytes 8..12 arrive.
	data := riff("WEBP")
	info, err := DetectReader(iotest.OneByteReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatalf("DetectReader: %v", err)
	}
	assertInfo(t, info, Image, FormatWebP, "image/webp")
}

func TestDetectReader_LargerThanWindow(t *testing.T) {
	t.Parallel()
	// DetectReader reads at most maxDetectBytes; an 8KB text stream still
	// classifies as text from its prefix.
	info, err := DetectReader(bytes.NewReader(bytes.Repeat([]byte{'Z'}, 8192)))
	if err != nil {
		t.Fatalf("DetectReader: %v", err)
	}
	assertInfo(t, info, Text, FormatText, "text/plain")
}

func TestDetectFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.png")
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if err := os.WriteFile(path, png, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := DetectFile(path)
	if err != nil {
		t.Fatalf("DetectFile: %v", err)
	}
	assertInfo(t, info, Image, FormatPNG, "image/png")
}

func TestDetectFile_NotFound(t *testing.T) {
	t.Parallel()
	if _, err := DetectFile("/nonexistent/path/file.bin"); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestDetectFile_Empty(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := DetectFile(path); err == nil {
		t.Error("expected error for empty file")
	}
}

func TestType_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		t    Type
		want string
	}{
		{Unknown, "unknown"},
		{Video, "video"},
		{Audio, "audio"},
		{Image, "image"},
		{Text, "text"},
		{Type(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("Type(%d).String() = %q, want %q", tt.t, got, tt.want)
		}
	}
}

func assertInfo(t *testing.T, got Info, wantType Type, wantFormat Format, wantMime string) {
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

// FuzzDetect exercises the byte-signature detector against arbitrary input to
// prove it never panics or reads out of bounds, and that its result is
// internally consistent (a classified type always carries a non-empty format).
func FuzzDetect(f *testing.F) {
	seeds := [][]byte{
		nil,
		{},
		{0x00},
		{0xFF, 0xD8, 0xFF, 0xE0},
		{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
		[]byte("GIF89a"),
		[]byte("RIFFxxxxWEBP"),
		[]byte("RIFFxxxxWAVE"),
		[]byte("RIFFxxxxAVI "),
		{0x1A, 0x45, 0xDF, 0xA3},
		[]byte("ftyp"),
		[]byte("....ftypisom"),
		[]byte("Hello, plain text world.\n"),
		{0x47},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		info := Detect(data)

		switch info.Type {
		case Unknown:
			if info.Format != FormatUnknown {
				t.Errorf("Unknown type carried format %q", info.Format)
			}
		case Video, Audio, Image, Text:
			if info.Format == FormatUnknown {
				t.Errorf("classified %v with empty format for %q", info.Type, data)
			}
			if info.MimeType == "" {
				t.Errorf("classified %v without mime type for %q", info.Type, data)
			}
		default:
			t.Errorf("unexpected type %d", info.Type)
		}

		// DetectReader over the same bytes must agree with Detect for non-empty
		// input (it reads up to maxDetectBytes, matching Detect's window).
		if len(data) > 0 {
			got, err := DetectReader(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("DetectReader errored on non-empty input: %v", err)
			}
			if len(data) <= maxDetectBytes && got != info {
				t.Errorf("DetectReader %+v != Detect %+v", got, info)
			}
		}
	})
}
