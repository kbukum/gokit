package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/image/webp"
)

// Defaults applied by [VerifyImage] when an [ImageLimits] field is zero.
const (
	DefaultMaxImageBytes     = 10 << 20
	DefaultMaxImageDimension = 16384
	DefaultMaxImagePixels    = 40_000_000
)

// VerifiableImageFormats are the formats [VerifyImage] can check: the still-image formats that hold no script.
var VerifiableImageFormats = []Format{FormatPNG, FormatJPEG, FormatWebP}

// ErrImageRejected matches every [*ImageError].
var ErrImageRejected = errors.New("media: image rejected")

// ImageRejection says why [VerifyImage] refused an image.
type ImageRejection string

// The reasons [VerifyImage] refuses an image.
const (
	RejectedEmpty        ImageRejection = "empty"         // No bytes
	RejectedTooLarge     ImageRejection = "too_large"     // More bytes than MaxBytes
	RejectedFormat       ImageRejection = "format"        // Not an allowed still-image format, judged from the bytes
	RejectedMalformed    ImageRejection = "malformed"     // The bytes break the format's structure or do not decode
	RejectedTrailingData ImageRejection = "trailing_data" // Bytes follow the image's end, as in a polyglot file
	RejectedDimensions   ImageRejection = "dimensions"    // Width, height or pixel count outside the limits
)

// ImageError is a refused image. It carries the reason and a short detail, never the image bytes.
type ImageError struct {
	Reason ImageRejection
	Detail string
}

func (e *ImageError) Error() string {
	return fmt.Sprintf("media: image rejected (%s): %s", e.Reason, e.Detail)
}

// Is reports whether target is [ErrImageRejected].
func (e *ImageError) Is(target error) bool { return target == ErrImageRejected }

func reject(reason ImageRejection, format string, args ...any) error {
	return &ImageError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// ImageLimits bounds what [VerifyImage] accepts. A zero field takes its default.
type ImageLimits struct {
	// Formats lists the accepted formats, a subset of [VerifiableImageFormats]. Empty means all of them.
	Formats []Format
	// MaxBytes bounds the encoded size. Zero means [DefaultMaxImageBytes].
	MaxBytes int64
	// MaxWidth and MaxHeight bound each side in pixels. Zero means [DefaultMaxImageDimension].
	MaxWidth, MaxHeight int
	// MaxPixels bounds width × height, at most [MaxDecodePixels]. Zero means [DefaultMaxImagePixels].
	MaxPixels int64
}

func (l ImageLimits) normalized() (ImageLimits, error) {
	if l.MaxBytes < 0 || l.MaxWidth < 0 || l.MaxHeight < 0 || l.MaxPixels < 0 || l.MaxPixels > MaxDecodePixels {
		return l, errors.New("media: image limits must not be negative, and MaxPixels must not exceed MaxDecodePixels")
	}
	for _, f := range l.Formats {
		if !slices.Contains(VerifiableImageFormats, f) {
			return l, fmt.Errorf("media: cannot verify image format %q", f)
		}
	}
	if len(l.Formats) == 0 {
		l.Formats = VerifiableImageFormats
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = DefaultMaxImageBytes
	}
	if l.MaxWidth == 0 {
		l.MaxWidth = DefaultMaxImageDimension
	}
	if l.MaxHeight == 0 {
		l.MaxHeight = DefaultMaxImageDimension
	}
	if l.MaxPixels == 0 {
		l.MaxPixels = DefaultMaxImagePixels
	}
	return l, nil
}

// VerifiedImage is an image [VerifyImage] accepted.
type VerifiedImage struct {
	Format   Format
	MimeType string
	Width    int
	Height   int
}

// VerifyImage checks untrusted bytes are one allowed still image and nothing else. The format comes from the bytes,
// never from a name or a declared type, so SVG, HTML and any other script-carrying format is refused. The
// dimensions are checked against limits from the header, before any pixel is decoded. Each image is then fully
// decoded. WebP also has its RIFF structure and frame header checked. Any byte
// past the image's end is refused, so a file that is also a zip or an HTML page does not pass.
//
// A refused image returns an [*ImageError] matching [ErrImageRejected]; invalid limits return a plain error.
func VerifyImage(data []byte, limits ImageLimits) (VerifiedImage, error) {
	l, err := limits.normalized()
	if err != nil {
		return VerifiedImage{}, err
	}
	if len(data) == 0 {
		return VerifiedImage{}, reject(RejectedEmpty, "no bytes")
	}
	if int64(len(data)) > l.MaxBytes {
		return VerifiedImage{}, reject(RejectedTooLarge, "%d bytes exceeds %d", len(data), l.MaxBytes)
	}
	info := Detect(data)
	if info.Type != Image || !slices.Contains(l.Formats, info.Format) {
		return VerifiedImage{}, reject(RejectedFormat, "the bytes are not an allowed image format")
	}
	var w, h int
	switch info.Format {
	case FormatWebP:
		w, h, err = webpSize(data)
		if err != nil {
			return VerifiedImage{}, err
		}
		if sizeErr := l.checkSize(w, h); sizeErr != nil {
			return VerifiedImage{}, sizeErr
		}
		decoded, decodeErr := webp.Decode(bytes.NewReader(data))
		if decodeErr != nil {
			return VerifiedImage{}, reject(RejectedMalformed, "WebP does not decode")
		}
		if decoded.Bounds().Dx() != w || decoded.Bounds().Dy() != h {
			return VerifiedImage{}, reject(RejectedMalformed, "WebP decoded size differs from its header")
		}
	default:
		w, h, err = decodeStill(data, info.Format, l)
		if err != nil {
			return VerifiedImage{}, err
		}
	}
	return VerifiedImage{Format: info.Format, MimeType: info.MimeType, Width: w, Height: h}, nil
}

func (l ImageLimits) checkSize(w, h int) error {
	if w <= 0 || h <= 0 || w > l.MaxWidth || h > l.MaxHeight || int64(w) > l.MaxPixels/int64(h) {
		return reject(RejectedDimensions, "%dx%d is outside the limits", w, h)
	}
	return nil
}

// decodeStill checks a PNG or JPEG's structure ends at the last byte, bounds its header's size, then decodes it.
func decodeStill(data []byte, format Format, l ImageLimits) (width, height int, err error) {
	var end int
	if format == FormatPNG {
		end, err = pngEnd(data)
	} else {
		end, err = jpegEnd(data)
	}
	if err != nil {
		return 0, 0, err
	}
	if end != len(data) {
		return 0, 0, reject(RejectedTrailingData, "%d bytes follow the image", len(data)-end)
	}
	cfg, _, err := DecodeConfig(data)
	if err != nil {
		return 0, 0, reject(RejectedMalformed, "header does not decode")
	}
	if err := l.checkSize(cfg.Width, cfg.Height); err != nil {
		return 0, 0, err
	}
	if _, _, err := Decode(data); err != nil {
		return 0, 0, reject(RejectedMalformed, "pixels do not decode")
	}
	return cfg.Width, cfg.Height, nil
}

// pngEnd walks the PNG chunks and returns the offset just past IEND.
func pngEnd(data []byte) (int, error) {
	off := len(pngSignature)
	for first := true; ; first = false {
		if len(data)-off < 12 {
			return 0, reject(RejectedMalformed, "PNG ends before IEND")
		}
		size := int64(binary.BigEndian.Uint32(data[off:]))
		kind := string(data[off+4 : off+8])
		if first && kind != "IHDR" {
			return 0, reject(RejectedMalformed, "PNG does not start with IHDR")
		}
		next := int64(off) + 12 + size
		if next > int64(len(data)) {
			return 0, reject(RejectedMalformed, "PNG chunk %q overruns the data", kind)
		}
		off = int(next)
		if kind == "IEND" {
			return off, nil
		}
	}
}

// jpegEnd walks the JPEG segments and entropy-coded data and returns the offset just past the first EOI marker.
func jpegEnd(data []byte) (int, error) {
	off := 2
	for {
		for off < len(data) && data[off] == 0xFF && off+1 < len(data) && data[off+1] == 0xFF {
			off++ // Fill bytes before a marker
		}
		if len(data)-off < 2 || data[off] != 0xFF {
			return 0, reject(RejectedMalformed, "JPEG marker expected")
		}
		marker := data[off+1]
		off += 2
		switch {
		case marker == 0xD9:
			return off, nil
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7):
			continue // Markers without a length
		}
		if len(data)-off < 2 {
			return 0, reject(RejectedMalformed, "JPEG segment length missing")
		}
		size := int(binary.BigEndian.Uint16(data[off:]))
		if size < 2 || off+size > len(data) {
			return 0, reject(RejectedMalformed, "JPEG segment overruns the data")
		}
		off += size
		if marker != 0xDA {
			continue
		}
		// Start of scan: skip entropy-coded data up to the next marker that is not a stuffed zero or a restart.
		for {
			i := bytes.IndexByte(data[off:], 0xFF)
			if i < 0 || off+i+1 >= len(data) {
				return 0, reject(RejectedMalformed, "JPEG scan ends without a marker")
			}
			off += i
			next := data[off+1]
			if next == 0x00 || next == 0xFF || (next >= 0xD0 && next <= 0xD7) {
				off++
				continue
			}
			break
		}
	}
}

// webpSize checks the RIFF container fills data exactly and returns the image size its frame header states.
func webpSize(data []byte) (width, height int, err error) {
	if len(data) < 20 || int64(binary.LittleEndian.Uint32(data[4:])) != int64(len(data))-8 {
		return 0, 0, reject(RejectedMalformed, "RIFF size does not match the data")
	}
	type riffChunk struct {
		kind    string
		payload []byte
	}
	var chunks []riffChunk
	for off := 12; off < len(data); {
		if len(data)-off < 8 {
			return 0, 0, reject(RejectedMalformed, "RIFF chunk header truncated")
		}
		size := int64(binary.LittleEndian.Uint32(data[off+4:]))
		end := int64(off) + 8 + size
		padded := end + size%2
		if padded > int64(len(data)) {
			return 0, 0, reject(RejectedMalformed, "RIFF chunk overruns the data")
		}
		chunks = append(chunks, riffChunk{kind: string(data[off : off+4]), payload: data[off+8 : end]})
		off = int(padded)
	}
	switch chunks[0].kind {
	case "VP8 ", "VP8L":
		return webpFrameSize(chunks[0].kind, chunks[0].payload)
	case "VP8X":
		p := chunks[0].payload
		if len(p) < 10 {
			return 0, 0, reject(RejectedMalformed, "VP8X header truncated")
		}
		if p[0]&0x02 != 0 {
			return 0, 0, reject(RejectedFormat, "animated WebP is not a still image")
		}
		w, h := 1+(int(p[4])|int(p[5])<<8|int(p[6])<<16), 1+(int(p[7])|int(p[8])<<8|int(p[9])<<16)
		for _, c := range chunks[1:] {
			if c.kind != "VP8 " && c.kind != "VP8L" {
				continue
			}
			fw, fh, err := webpFrameSize(c.kind, c.payload)
			if err != nil {
				return 0, 0, err
			}
			if fw != w || fh != h {
				return 0, 0, reject(RejectedMalformed, "WebP frame size differs from its canvas")
			}
			return w, h, nil
		}
		return 0, 0, reject(RejectedMalformed, "WebP has no image frame")
	default:
		return 0, 0, reject(RejectedMalformed, "WebP starts with an unknown chunk")
	}
}

func webpFrameSize(kind string, p []byte) (width, height int, err error) {
	if kind == "VP8L" {
		if len(p) < 5 || p[0] != 0x2f {
			return 0, 0, reject(RejectedMalformed, "VP8L header is invalid")
		}
		bits := binary.LittleEndian.Uint32(p[1:])
		if bits>>29 != 0 {
			return 0, 0, reject(RejectedMalformed, "VP8L version is unknown")
		}
		return 1 + int(bits&0x3fff), 1 + int(bits>>14&0x3fff), nil
	}
	if len(p) < 10 || p[0]&0x01 != 0 || !bytes.Equal(p[3:6], []byte{0x9d, 0x01, 0x2a}) {
		return 0, 0, reject(RejectedMalformed, "VP8 key frame header is invalid")
	}
	return int(binary.LittleEndian.Uint16(p[6:]) & 0x3fff), int(binary.LittleEndian.Uint16(p[8:]) & 0x3fff), nil
}
