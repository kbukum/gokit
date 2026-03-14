# media

Media type detection from content bytes using magic byte (file signature) matching. Zero external dependencies.

## Install

```bash
go get github.com/kbukum/gokit
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
    fmt.Println(info.Container) // QuickTime
}
```

## Detection Methods

```go
// From raw bytes
info := media.Detect(data)

// From an io.Reader (reads up to 4096 bytes)
info, err := media.DetectReader(reader)

// From a file path
info, err := media.DetectFile("/path/to/file.png")
```

## Supported Formats

### Video
| Format | Extension | Container |
|--------|-----------|-----------|
| MP4 | .mp4, .m4v | QuickTime |
| MOV | .mov | QuickTime |
| WebM | .webm | Matroska |
| MKV | .mkv | Matroska |
| AVI | .avi | RIFF |
| FLV | .flv | Flash |
| MPEG-TS | .ts | MPEG-TS |

### Audio
| Format | Extension | Container |
|--------|-----------|-----------|
| WAV | .wav | RIFF |
| FLAC | .flac | — |
| OGG | .ogg | OGG |
| MP3 | .mp3 | — |
| AAC | .aac | ADTS |
| MIDI | .mid | — |
| AIFF | .aiff | — |
| M4A | .m4a | QuickTime |

### Image
| Format | Extension |
|--------|-----------|
| JPEG | .jpg, .jpeg |
| PNG | .png |
| GIF | .gif |
| WebP | .webp |
| BMP | .bmp |
| TIFF | .tif, .tiff |
| ICO | .ico |
| AVIF | .avif |
| HEIF | .heic |

### Text
Detected via UTF-8 validation and printable character ratio (≥95%).

## Key Types

| Name | Description |
|------|-------------|
| `Type` | Media category: `Unknown`, `Video`, `Audio`, `Image`, `Text` |
| `Info` | Detection result: Type, Format, MimeType, Container |
| `Detect([]byte)` | Detect from raw bytes |
| `DetectReader(io.Reader)` | Detect from a reader |
| `DetectFile(string)` | Detect from a file path |

---

[⬅ Back to main README](../README.md)
