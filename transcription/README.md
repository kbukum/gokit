# transcription

Transcription provider framework with segment-level output and provider registry. Ships with a Whisper implementation.

## Install

```bash
go get github.com/skillsenselab/gokit/transcription@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/transcription"
    "github.com/skillsenselab/gokit/transcription/whisper"
)

// Create a Whisper provider
p := whisper.NewProvider(whisper.Config{
    URL:      "http://localhost:9000",
    Model:    "large-v3",
    Language: "en",
})

// Transcribe audio
resp, _ := p.Transcribe(ctx, transcription.TranscriptionRequest{
    AudioPath: "/data/recording.wav",
    Language:  "en",
})

fmt.Println(resp.Text)
fmt.Printf("Duration: %.1fs, Segments: %d\n", resp.Duration, len(resp.Segments))
for _, seg := range resp.Segments {
    fmt.Printf("[%.1f-%.1f] %s\n", seg.Start, seg.End, seg.Text)
}
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Provider` | Interface — `Transcribe`, `Name`, `IsAvailable` |
| `TranscriptionRequest` | AudioPath, Language, Model, Format |
| `TranscriptionResponse` | Text, Segments, Duration, Language |
| `Segment` | Start, End, Text, Speaker |
| `NewRegistry()` | Create a provider registry |
| `NewManager(opts...)` | Create a provider manager with selector |

### `transcription/whisper`

| Symbol | Description |
|---|---|
| `NewProvider(cfg)` | Create a Whisper transcription provider |
| `Factory()` | Provider factory for registry registration |
| `Config` | URL, Model, Language, Device, ComputeType, Timeout |

---

[← Back to main gokit README](../README.md)
