// Package media provides light, dependency-free media handling for Go: byte-signature type/format detection, container metadata reads, cheap pure-Go image ops, and pure-Go time/spatial vocabulary plus subtitle (SRT/WebVTT) parsing and serialization.
//
// It is the light mirror of rskit's media capability. The surface concepts read in parallel — [Type]/[Info], the [Format]/[FormatInfo] catalog, an injected [Registry], the [Prober] abstraction, [Timestamp]/[TimeRange]/[Resolution], and [SubtitleTrack] — but heavy audio/video/matrix/DSP processing (transcoding, filters, resampling) is rskit-only by design: a capability decision, not a parity gap. gokit ships no cgo, no ffmpeg, and no Go DSP/matrix code; the heavy path is rskit or an external service, never a Go reimplementation.
//
// The zero-value entry points classify content without decoding it:
//
//	info := media.Detect(data)
//	if info.Type == media.Video {
//	    fmt.Println("Video format:", info.Format)
//	}
//
// A [Registry] wires the format catalog together with injected [Prober] backends to enrich detection with metadata such as a [Resolution]:
//
//	reg := media.NewRegistry(media.WithImageProber())
//	meta := reg.Probe(data) // Info plus Resolution for stdlib-decodable images
//
// [SubtitleTrack] parses and serializes SRT and WebVTT, with time-shifting and
// range filtering built on the [Timestamp]/[TimeRange] types:
//
//	track, err := media.ParseSRT(srtText)
//	track.Shift(500 * time.Millisecond)
//	vtt := track.VTT()
package media
