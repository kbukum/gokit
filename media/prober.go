package media

import "errors"

// ErrUnsupported is returned by a [Prober] when it does not recognize the content it was given.
// It lets a [Registry] skip a prober and try the next one.
var ErrUnsupported = errors.New("media: unsupported content for prober")

// Metadata is the enriched result of probing content:
// the detection [Info] plus any lightweight properties a backend could extract without full processing.
//
// It is the light-kit parallel of rskit's media metadata;
// the light surface carries only cheaply derivable fields (currently image/frame dimensions).
type Metadata struct {
	Info
	// Resolution is the pixel dimensions for images or video frames,
	// or the zero [Resolution] when the probing backend cannot determine them.
	Resolution Resolution `json:"resolution,omitzero"`
}

// Prober inspects the leading bytes of content
// and enriches its detection with lightweight properties (e.g. pixel dimensions) for the formats it understands.
// It is the light-kit parallel of rskit's MediaProbe abstraction —
// a typed seam that backends implement,
// injected into a [Registry] rather than registered through package globals.
//
// A [Registry] treats signature detection as authoritative for classification:
// it derives [Metadata.Info] from its own [Detect]
// and uses only the extra fields (such as [Metadata.Resolution]) a prober returns.
// The [Info] a prober puts on its returned [Metadata] is therefore ignored by [Registry.Probe].
//
// A prober returns [ErrUnsupported] (wrapped is fine) when it does not recognize the content,
// so a [Registry] can skip it and try the next prober.
type Prober interface {
	Probe(data []byte) (Metadata, error)
}
