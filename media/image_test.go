package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// encodePNG builds a solid-color PNG of the given size for use as test input.
func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodeGIF(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeConfig_StdlibFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		data   []byte
		format Format
	}{
		{"png", encodePNG(t, 20, 10), FormatPNG},
		{"jpeg", encodeJPEG(t, 16, 8), FormatJPEG},
		{"gif", encodeGIF(t, 12, 6), FormatGIF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, format, err := DecodeConfig(tt.data)
			if err != nil {
				t.Fatalf("DecodeConfig: %v", err)
			}
			if format != tt.format {
				t.Errorf("format = %q, want %q", format, tt.format)
			}
			if cfg.Width == 0 || cfg.Height == 0 {
				t.Errorf("dimensions unset: %+v", cfg)
			}
		})
	}
}

func TestDecodeConfig_UnsupportedContent(t *testing.T) {
	t.Parallel()
	_, _, err := DecodeConfig([]byte("not an image"))
	if err == nil {
		t.Fatal("expected error for non-image content")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func TestDecode_ReturnsImageAndFormat(t *testing.T) {
	t.Parallel()
	img, format, err := Decode(encodePNG(t, 8, 8))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if format != FormatPNG {
		t.Errorf("format = %q, want png", format)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 8 {
		t.Errorf("bounds = %v, want 8x8", img.Bounds())
	}
}

func TestDecodeConfig_CorruptSupportedFormatPreservesCause(t *testing.T) {
	t.Parallel()
	// A truncated PNG carries a valid signature, so the failure is a decode
	// error on a supported format, not an unsupported format.
	full := encodePNG(t, 8, 8)
	// Keep the 8-byte PNG signature (so the format is recognized) but cut off
	// the IHDR header so decoding the config fails on a supported format.
	truncated := full[:12]
	_, _, err := DecodeConfig(truncated)
	if err == nil {
		t.Fatal("expected error for truncated PNG")
	}
	if errors.Is(err, ErrUnsupported) {
		t.Errorf("truncated PNG should not wrap ErrUnsupported, got %v", err)
	}
}

func TestDecode_Unsupported(t *testing.T) {
	t.Parallel()
	if _, _, err := Decode([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("expected error decoding garbage")
	}
}

func TestDecode_RejectsOversizedImage(t *testing.T) {
	t.Parallel()
	// A valid PNG header declaring 20000x20000 (4e8 px) exceeds MaxDecodePixels;
	// Decode must reject it via the header before allocating the pixel buffer.
	data := craftPNGHeader(t, 20000, 20000)
	_, _, err := Decode(data)
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("Decode oversized = %v, want ErrImageTooLarge", err)
	}
}

func TestStdlibFormat_UnknownPassesThrough(t *testing.T) {
	t.Parallel()
	if got := stdlibFormat("webp"); got != Format("webp") {
		t.Errorf("stdlibFormat(webp) = %q, want passthrough", got)
	}
}

// craftPNGHeader builds a PNG signature + IHDR chunk declaring the given
// dimensions, with a correct CRC so image.DecodeConfig accepts it. The pixel
// data is omitted, which is sufficient for header-only inspection.
func craftPNGHeader(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	buf.Write(chunk)
	_ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestThumbnail_DownscalesPreservingAspect(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	thumb := Thumbnail(src, 20, 20)
	if thumb.Bounds().Dx() != 20 || thumb.Bounds().Dy() != 10 {
		t.Errorf("thumbnail bounds = %v, want 20x10", thumb.Bounds())
	}
}

func TestThumbnail_NeverUpscales(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	thumb := Thumbnail(src, 100, 100)
	if thumb.Bounds().Dx() != 10 || thumb.Bounds().Dy() != 10 {
		t.Errorf("thumbnail bounds = %v, want unchanged 10x10", thumb.Bounds())
	}
}

func TestThumbnail_ZeroBoundsAndInputs(t *testing.T) {
	t.Parallel()
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if got := Thumbnail(empty, 10, 10); got.Bounds().Dx() != 0 {
		t.Errorf("expected empty thumbnail for empty source, got %v", got.Bounds())
	}
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if got := Thumbnail(src, 0, 10); got.Bounds().Dx() != 0 {
		t.Errorf("expected empty thumbnail for zero maxW, got %v", got.Bounds())
	}
}

func TestThumbnail_ExtremeDownscaleClampsToOne(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 1000, 10))
	thumb := Thumbnail(src, 1, 1)
	if thumb.Bounds().Dx() < 1 || thumb.Bounds().Dy() < 1 {
		t.Errorf("thumbnail collapsed below 1px: %v", thumb.Bounds())
	}
}

func TestCrop_SubImageView(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	cropped := Crop(src, image.Rect(10, 10, 40, 30))
	if cropped.Bounds().Dx() != 30 || cropped.Bounds().Dy() != 20 {
		t.Errorf("crop bounds = %v, want 30x20", cropped.Bounds())
	}
}

func TestCrop_IntersectsWithSourceBounds(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	cropped := Crop(src, image.Rect(10, 10, 100, 100))
	if cropped.Bounds().Dx() != 10 || cropped.Bounds().Dy() != 10 {
		t.Errorf("crop bounds = %v, want clamped 10x10", cropped.Bounds())
	}
}

// stubImage has no SubImage method, exercising Crop's copy fallback.
type stubImage struct{ image.Image }

func TestCrop_CopyFallbackWithoutSubImage(t *testing.T) {
	t.Parallel()
	src := stubImage{image.NewRGBA(image.Rect(0, 0, 10, 10))}
	cropped := Crop(src, image.Rect(2, 2, 6, 8))
	if cropped.Bounds().Dx() != 4 || cropped.Bounds().Dy() != 6 {
		t.Errorf("crop bounds = %v, want 4x6", cropped.Bounds())
	}
}
