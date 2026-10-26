package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

// webpLossless1x1 is a real 1×1 lossless WebP (RIFF, VP8L chunk, one pad byte).
var webpLossless1x1 = []byte{
	'R', 'I', 'F', 'F', 0x1a, 0, 0, 0, 'W', 'E', 'B', 'P',
	'V', 'P', '8', 'L', 0x0d, 0, 0, 0,
	0x2f, 0, 0, 0, 0x10, 0x07, 0x10, 0x11, 0x11, 0x88, 0x88, 0xfe, 0x07, 0,
}

// webpLossy builds a RIFF WebP whose VP8 key frame states w×h.
func webpLossy(w, h uint16) []byte {
	frame := []byte{0x30, 0x01, 0x00, 0x9d, 0x01, 0x2a, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(frame[6:], w)
	binary.LittleEndian.PutUint16(frame[8:], h)
	return webpRIFF(riffChunk("VP8 ", frame))
}

// webpExtended builds a RIFF WebP whose VP8X header states a w×h canvas.
func webpExtended(w, h uint32) []byte {
	header := make([]byte, 10)
	put24 := func(b []byte, v uint32) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }
	put24(header[4:], w-1)
	put24(header[7:], h-1)
	bits := make([]byte, 5, 8)
	bits[0] = 0x2f
	binary.LittleEndian.PutUint32(bits[1:], (w-1)|(h-1)<<14)
	return webpRIFF(riffChunk("VP8X", header), riffChunk("VP8L", append(bits, 0, 0, 0)))
}

func riffChunk(fourcc string, payload []byte) []byte {
	c := append([]byte(fourcc), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(c[4:], uint32(len(payload)))
	c = append(c, payload...)
	if len(payload)%2 == 1 {
		c = append(c, 0)
	}
	return c
}

func webpRIFF(chunks ...[]byte) []byte {
	out := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, c := range chunks {
		out = append(out, c...)
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

func reason(t *testing.T, err error) ImageRejection {
	t.Helper()
	var ie *ImageError
	if !errors.As(err, &ie) {
		t.Fatalf("error %v is not an *ImageError", err)
	}
	return ie.Reason
}

// Encoded from solid red PNGs with cwebp; these contain real image payloads, not just frame headers.
func encodedWebP(t *testing.T, encoded string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validExtendedWebP(t *testing.T) []byte {
	t.Helper()
	frame := encodedWebP(t, "UklGRjgAAABXRUJQVlA4TCwAAAAv/8H/AAcQ/Y/+BwQCyf7iIxTR/4z//Oc///nPf/7zn//85z//+c9//vN/AQ==")
	return webpRIFF(riffChunk("VP8X", []byte{0, 0, 0, 0, 255, 1, 0, 255, 3, 0}), frame[12:])
}

func TestVerifyImageAccepts(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		data          []byte
		format        Format
		mime          string
		width, height int
	}{
		"png":           {encodePNG(t, 3, 2), FormatPNG, "image/png", 3, 2},
		"jpeg":          {encodeJPEG(t, 4, 5), FormatJPEG, "image/jpeg", 4, 5},
		"webp lossless": {webpLossless1x1, FormatWebP, "image/webp", 1, 1},
		"webp lossy":    {encodedWebP(t, "UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoQAAkAAUAmJaACdLoB+AADsAD+8ut//NgVzXPv9//S4P0uD9Lg/9KQAAA="), FormatWebP, "image/webp", 16, 9},
		"webp extended": {validExtendedWebP(t), FormatWebP, "image/webp", 512, 1024},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := VerifyImage(tc.data, ImageLimits{})
			if err != nil {
				t.Fatalf("VerifyImage: %v", err)
			}
			want := VerifiedImage{Format: tc.format, MimeType: tc.mime, Width: tc.width, Height: tc.height}
			if got != want {
				t.Fatalf("VerifyImage = %+v, want %+v", got, want)
			}
			if tc.format == FormatWebP {
				config, format, err := DecodeConfig(tc.data)
				if err != nil || format != FormatWebP || config.Width != tc.width || config.Height != tc.height {
					t.Fatalf("WebP config = %+v, %v, %v", config, format, err)
				}
			}
		})
	}
}

func TestVerifyImageRejects(t *testing.T) {
	t.Parallel()
	pngData := encodePNG(t, 3, 2)
	jpegData := encodeJPEG(t, 4, 5)
	badRIFF := append([]byte{}, webpLossless1x1...)
	binary.LittleEndian.PutUint32(badRIFF[4:], 100)
	badVP8 := webpLossy(16, 9)
	badVP8[23] = 0 // break the key frame start code
	for name, tc := range map[string]struct {
		data   []byte
		limits ImageLimits
		want   ImageRejection
	}{
		"empty":                {nil, ImageLimits{}, RejectedEmpty},
		"svg":                  {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ImageLimits{}, RejectedFormat},
		"html":                 {[]byte("<!doctype html><script>alert(1)</script>"), ImageLimits{}, RejectedFormat},
		"gif by default":       {[]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), ImageLimits{}, RejectedFormat},
		"png not allowed":      {pngData, ImageLimits{Formats: []Format{FormatJPEG}}, RejectedFormat},
		"too many bytes":       {pngData, ImageLimits{MaxBytes: 10}, RejectedTooLarge},
		"png then zip":         {append(append([]byte{}, pngData...), "PK\x03\x04polyglot"...), ImageLimits{}, RejectedTrailingData},
		"png then html":        {append(append([]byte{}, pngData...), "<script>"...), ImageLimits{}, RejectedTrailingData},
		"jpeg then html":       {append(append([]byte{}, jpegData...), "<html>"...), ImageLimits{}, RejectedTrailingData},
		"truncated png":        {pngData[:len(pngData)-20], ImageLimits{}, RejectedMalformed},
		"truncated jpeg":       {jpegData[:len(jpegData)/2], ImageLimits{}, RejectedMalformed},
		"webp riff size":       {badRIFF, ImageLimits{}, RejectedMalformed},
		"webp then data":       {append(append([]byte{}, webpLossless1x1...), "PK"...), ImageLimits{}, RejectedMalformed},
		"webp bad frame":       {badVP8, ImageLimits{}, RejectedMalformed},
		"webp no frame data":   {webpLossy(16, 9), ImageLimits{}, RejectedMalformed},
		"webp too many pixels": {webpExtended(16000, 4000), ImageLimits{}, RejectedDimensions},
		"webp canvas mismatch": {webpRIFF(riffChunk("VP8X", []byte{0, 0, 0, 0, 1, 0, 0, 1, 0, 0}), webpLossless1x1[12:]), ImageLimits{}, RejectedMalformed},
		"webp header only":     {webpRIFF(riffChunk("VP8X", make([]byte, 10))), ImageLimits{}, RejectedMalformed},
		"webp animated":        {webpRIFF(riffChunk("VP8X", []byte{0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0}), webpLossless1x1[12:]), ImageLimits{}, RejectedFormat},
		"webp unknown first":   {webpRIFF(riffChunk("JUNK", []byte{1, 2})), ImageLimits{}, RejectedMalformed},
		"png past max width":   {pngData, ImageLimits{MaxWidth: 2}, RejectedDimensions},
		"png past max pixels":  {pngData, ImageLimits{MaxPixels: 5}, RejectedDimensions},
		"webp past max height": {webpExtended(10, 10), ImageLimits{MaxHeight: 9}, RejectedDimensions},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := VerifyImage(tc.data, tc.limits)
			if got := reason(t, err); got != tc.want {
				t.Fatalf("reason = %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

// A PNG whose header states a huge canvas is refused before any pixel is decoded.
func TestVerifyImageRefusesDecompressionBomb(t *testing.T) {
	t.Parallel()
	data := encodePNG(t, 1, 1)
	binary.BigEndian.PutUint32(data[16:], 50000)
	binary.BigEndian.PutUint32(data[20:], 50000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	if got := reason(t, mustFail(VerifyImage(data, ImageLimits{}))); got != RejectedDimensions {
		t.Fatalf("reason = %q, want %q", got, RejectedDimensions)
	}
}

func TestImageErrorMessageNamesReason(t *testing.T) {
	t.Parallel()
	_, err := VerifyImage([]byte("plain text"), ImageLimits{})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte(string(RejectedFormat))) {
		t.Fatalf("error %v should name its reason", err)
	}
	if !errors.Is(err, ErrImageRejected) {
		t.Fatalf("error %v should match ErrImageRejected", err)
	}
}

func TestImageLimitsValidate(t *testing.T) {
	t.Parallel()
	for _, l := range []ImageLimits{{MaxBytes: -1}, {MaxWidth: -1}, {MaxHeight: -1}, {MaxPixels: -1}, {Formats: []Format{FormatTIFF}}} {
		if _, err := VerifyImage(encodePNG(t, 1, 1), l); err == nil || errors.Is(err, ErrImageRejected) {
			t.Fatalf("limits %+v: got %v, want a configuration error", l, err)
		}
	}
}

func mustFail(_ VerifiedImage, err error) error { return err }

func FuzzVerifyImage(f *testing.F) {
	f.Add(webpLossless1x1)
	f.Add(webpLossy(2, 2))
	f.Add(webpExtended(2, 2))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := VerifyImage(data, ImageLimits{MaxPixels: 1 << 20})
		if err == nil && (v.Width <= 0 || v.Height <= 0 || int64(v.Width)*int64(v.Height) > 1<<20) {
			t.Fatalf("accepted %+v outside its limits", v)
		}
	})
}
