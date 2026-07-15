# media

Light, dependency-free media handling for Go: byte-signature type/format
detection, container metadata reads, cheap pure-Go image ops, pure-Go
time/spatial vocabulary, and subtitle (SRT/WebVTT) parsing. Zero external
dependencies.

`media` is a standalone module (`github.com/kbukum/gokit/media`). It is the
**light** mirror of rskit's media capability: Go handles metadata, format and
container inspection, cheap image ops, time/spatial math, and subtitle parsing;
**heavy audio/video/matrix/DSP processing stays rskit-only by design**. This is a
capability decision, not a parity gap — see
[Capability split](#capability-split-light-by-design).

## Install

```bash
go get github.com/kbukum/gokit/media
```

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/kbukum/gokit/media"
)

func main() {
    data := loadFileBytes("sample.mp4")

    info := media.Detect(data)
    fmt.Println(info.Type)      // video
    fmt.Println(info.Format)    // mp4
    fmt.Println(info.MimeType)  // video/mp4
    fmt.Println(info.Container) // ISO BMFF
}
```

## Detection

```go
// From raw bytes
info := media.Detect(data)

// From an io.Reader (reads up to 4096 bytes)
info, err := media.DetectReader(reader)

// From a file path
info, err := media.DetectFile("/path/to/file.png")
```

## Registry & probing

A `Registry` ties the format catalog together with injected `Prober` backends.
It is constructed explicitly with functional options — no package globals, no
`init()` side effects.

```go
reg := media.NewRegistry(media.WithImageProber())

meta := reg.Probe(pngBytes)
fmt.Println(meta.Type, meta.Format)              // image png
fmt.Println(meta.Resolution.Width, meta.Resolution.Height) // 1920 1080 (JPEG/PNG/GIF)

fi, ok := reg.Lookup(media.FormatMP4)  // catalog entry
formats := reg.Formats()               // full sorted catalog
```

Implement `Prober` to add your own metadata backend:

```go
type Prober interface {
    Probe(data []byte) (media.Metadata, error) // return media.ErrUnsupported when not yours
}

reg := media.NewRegistry(media.WithProber(myProber{}))
```

## Image ops (pure Go, stdlib formats)

Cheap operations over the stdlib-decodable formats (JPEG, PNG, GIF):

```go
cfg, format, err := media.DecodeConfig(data) // dimensions, no full decode
img, format, err := media.Decode(data)       // full decode
thumb := media.Thumbnail(img, 256, 256)       // nearest-neighbor, never upscales
view := media.Crop(img, image.Rect(0, 0, 100, 100))
```

Formats outside the stdlib decoders (WebP, TIFF, HEIF, AVIF) are **detected** but
not decoded here; decoding/processing those is rskit's or an external service's job.
`Decode` reads the header first and rejects inputs above `MaxDecodePixels` to
bound memory on untrusted content (decompression bombs).

## Time & spatial types (pure Go)

Value types for media time and geometry math — no backend required:

```go
r := media.TimeRangeFromMillis(1000, 2500)
r.Duration()               // 1.5s
r.Overlaps(other)          // bool
r = r.Shift(500 * time.Millisecond)

res := media.Resolution1080p()
res.AspectRatio()          // 16, 9
res.ScaleToFit(1280, 1280) // 1280x720, aspect preserved
fps := media.NTSC30().Float() // 29.97
```

## Subtitles (SRT / WebVTT)

Pure-Go parsing, serialization, time-shifting, and range filtering. HTML tags are
stripped and (for VTT) entities decoded; malformed timestamps fail closed with
`ErrInvalidSubtitle`.

```go
track, err := media.ParseSRT(srtText)
track.Shift(-500 * time.Millisecond)      // resync
clip := track.InRange(media.TimeRangeFromMillis(0, 30_000))
vtt := clip.VTT()                         // convert SRT → WebVTT
```

## Capability split (light by design)

| Capability | gokit `media` | rskit `media` |
|---|---|---|
| Type/format detection (magic bytes) | ✅ | ✅ |
| Container/metadata read (dimensions) | ✅ (stdlib image) | ✅ |
| Cheap image ops (crop, nearest-neighbor thumbnail) | ✅ | ✅ |
| Time/spatial vocabulary (Timestamp, TimeRange, Resolution, FrameRate) | ✅ | ✅ |
| Subtitle parse/serialize (SRT, WebVTT) | ✅ | ✅ |
| High-quality resampling / filters | ➖ | ✅ |
| Audio/video transcoding, ffmpeg | ➖ (by design) | ✅ |
| Matrix/DSP, scene detection, waveforms | ➖ (by design) | ✅ |
| Codec/color/pipeline/output executor vocabulary | ➖ (backend-only) | ✅ |

The heavy path is **rskit or an external service**, never a Go reimplementation.
gokit deliberately has **no cgo, no ffmpeg, and no Go DSP/matrix code**. Backend-only
vocabulary (codec, color, filter graphs, pipeline/output configs) is intentionally
omitted: without a transcoding executor it would be dead surface. See
[`docs/parity-matrix.md`](../docs/parity-matrix.md).

## Supported formats (detection)

- **Video:** MP4, MOV, M4V, WebM, MKV, AVI, FLV, MPEG-TS
- **Audio:** WAV, FLAC, OGG, AAC, MP3, MIDI, AIFF, M4A
- **Image:** JPEG, PNG, GIF, WebP, BMP, TIFF, ICO, AVIF, HEIF
- **Text:** UTF-8 heuristic (≥95% printable)

## Key types

| Name | Description |
|------|-------------|
| `Type` | Media category: `Unknown`, `Video`, `Audio`, `Image`, `Text` |
| `Format` | Format identifier (e.g. `FormatMP4`, `FormatJPEG`) |
| `Info` | Detection result: Type, Format, MimeType, Container |
| `FormatInfo` | Catalog entry: Format, Type, Extension, MimeType, Container |
| `Metadata` | Probe result: `Info` plus `Resolution` |
| `Registry` | Injected format catalog + prober backends |
| `Prober` | Metadata backend abstraction |
| `Detect` / `DetectReader` / `DetectFile` | Signature detection entry points |
| `DecodeConfig` / `Decode` / `Thumbnail` / `Crop` | Pure-Go image ops |
| `Timestamp` / `TimeRange` / `Segment` | Time vocabulary and range math |
| `Resolution` / `FrameRate` | Spatial vocabulary (presets, aspect ratio, scaling) |
| `SubtitleTrack` / `ParseSRT` / `ParseVTT` | Subtitle parse/serialize (SRT, WebVTT) |

---

[⬅ Back to main README](../README.md)
