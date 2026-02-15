# diarization

Speaker diarization provider framework with segment-level output and provider registry. Ships with a Pyannote implementation.

## Install

```bash
go get github.com/skillsenselab/gokit/diarization@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/diarization"
    "github.com/skillsenselab/gokit/diarization/pyannote"
)

// Create a Pyannote provider
p := pyannote.NewProvider(pyannote.Config{
    BaseURL: "http://localhost:8388",
    Timeout: 300 * time.Second,
})

// Diarize audio
resp, _ := p.Diarize(ctx, diarization.DiarizationRequest{
    AudioPath:   "/data/meeting.wav",
    MinSpeakers: 2,
    MaxSpeakers: 5,
})

fmt.Printf("Detected %d speakers\n", resp.NumSpeakers)
for _, seg := range resp.Segments {
    fmt.Printf("[%.1f-%.1f] %s: %s\n", seg.Start, seg.End, seg.Speaker, seg.Text)
}
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Provider` | Interface — `Diarize`, `Name`, `IsAvailable` |
| `DiarizationRequest` | AudioPath, NumSpeakers, MinSpeakers, MaxSpeakers, Language |
| `DiarizationResponse` | Segments, NumSpeakers |
| `Segment` | Speaker, Start, End, Text |
| `NewRegistry()` | Create a provider registry |
| `NewManager(opts...)` | Create a provider manager with selector |

### `diarization/pyannote`

| Symbol | Description |
|---|---|
| `NewProvider(cfg)` | Create a Pyannote diarization provider |
| `Factory()` | Provider factory for registry registration |
| `Config` | BaseURL, Timeout |

---

[← Back to main gokit README](../README.md)
