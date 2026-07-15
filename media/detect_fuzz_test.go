package media

import (
	"bytes"
	"testing"
)

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
