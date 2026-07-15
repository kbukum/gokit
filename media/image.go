package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"

	// Register the stdlib image decoders so DecodeConfig/Decode recognize the
	// pure-Go formats the light kit supports. Heavier formats (webp, tiff, heif)
	// are detected but intentionally not decoded here — that stays rskit-only.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// MaxDecodePixels bounds the pixel count [Decode] will fully decode from
// untrusted input, guarding against decompression bombs (a tiny compressed file
// declaring enormous dimensions). It corresponds to roughly a 100-megapixel
// image; callers needing larger inputs must decode via the stdlib directly.
const MaxDecodePixels = 100_000_000

// ErrImageTooLarge is returned by [Decode] when the declared pixel count
// (width × height) exceeds [MaxDecodePixels].
var ErrImageTooLarge = errors.New("media: image exceeds maximum decode pixel count")

// DecodeConfig reads only the header of data and returns the image dimensions
// and detected [Format] without decoding the full pixel buffer.
//
// It supports the pure-Go stdlib formats (JPEG, PNG, GIF); other detected image
// formats wrap [ErrUnsupported]. Decode failures on a supported format (corrupt
// or truncated data) preserve the underlying cause instead. On any error the
// returned [Format] is a best-effort signature detection (via [Detect]), which
// may be [FormatUnknown] when the bytes match no known signature.
func DecodeConfig(data []byte) (cfg image.Config, format Format, err error) {
	cfg, name, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		detected := Detect(data).Format
		if errors.Is(err, image.ErrFormat) {
			return image.Config{}, detected, fmt.Errorf("%w: %w", ErrUnsupported, err)
		}
		return image.Config{}, detected, fmt.Errorf("media: decode config: %w", err)
	}
	return cfg, stdlibFormat(name), nil
}

// Decode fully decodes data into an [image.Image] using the stdlib decoders,
// returning the detected [Format]. It first reads the header and rejects inputs
// whose declared pixel count (width × height) exceeds [MaxDecodePixels] (wrapping
// [ErrImageTooLarge]) to bound memory use on untrusted content. Unrecognized
// formats wrap [ErrUnsupported]; decode failures on a supported format preserve
// the cause.
// On every error path the returned [Format] is the best-effort detected format
// (which may be [FormatUnknown]), never silently discarded.
func Decode(data []byte) (img image.Image, format Format, err error) {
	cfg, format, err := DecodeConfig(data)
	if err != nil {
		return nil, format, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxDecodePixels {
		return nil, format, fmt.Errorf("%w: %dx%d", ErrImageTooLarge, cfg.Width, cfg.Height)
	}
	img, name, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil, format, fmt.Errorf("%w: %w", ErrUnsupported, err)
		}
		return nil, format, fmt.Errorf("media: decode: %w", err)
	}
	return img, stdlibFormat(name), nil
}

// Crop returns the sub-image of src bounded by r. It is a cheap, allocation-free
// view when src supports SubImage (all stdlib image types do); otherwise the
// pixels are copied. r is intersected with the source bounds, and the result
// preserves the source coordinate space (its Bounds equal the intersected r)
// on both paths.
func Crop(src image.Image, r image.Rectangle) image.Image {
	r = r.Intersect(src.Bounds())
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := src.(subImager); ok {
		return si.SubImage(r)
	}
	dst := image.NewRGBA(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}

// Thumbnail downscales src to fit within maxW×maxH while preserving aspect
// ratio, using nearest-neighbor sampling. It never upscales: sources already
// within the bounds are returned unchanged. It is a deliberately cheap pure-Go
// operation; high-quality resampling belongs in rskit.
func Thumbnail(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || maxW <= 0 || maxH <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	if w <= maxW && h <= maxH {
		return src
	}

	scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	dw := max(1, int(float64(w)*scale))
	dh := max(1, int(float64(h)*scale))

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := b.Min.Y + y*h/dh
		for x := 0; x < dw; x++ {
			sx := b.Min.X + x*w/dw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// stdlibFormat maps an image/... decoder name to the light-kit [Format].
func stdlibFormat(name string) Format {
	switch name {
	case "jpeg":
		return FormatJPEG
	case "png":
		return FormatPNG
	case "gif":
		return FormatGIF
	default:
		return Format(name)
	}
}

// imageProber is the built-in [Prober] backend that reads image dimensions via
// the stdlib decoders. It is injected into a [Registry] with [WithImageProber].
type imageProber struct{}

// Probe returns image [Metadata] (with Width/Height) for stdlib-decodable
// images (JPEG, PNG, GIF). It returns an error for non-images and undecodable
// content so a [Registry] can fall back to signature-only detection.
// Classification comes from the signature detector, which is authoritative; the
// decoder contributes only the pixel dimensions.
func (imageProber) Probe(data []byte) (Metadata, error) {
	cfg, _, err := DecodeConfig(data)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{
		Info:       Detect(data),
		Resolution: Resolution{Width: cfg.Width, Height: cfg.Height},
	}, nil
}
