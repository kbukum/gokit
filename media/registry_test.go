package media

import (
	"testing"
)

func TestNewRegistry_SeedsCatalog(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	if _, ok := reg.Lookup(FormatMP4); !ok {
		t.Error("expected mp4 in default catalog")
	}
	if got := len(reg.Formats()); got != len(knownFormats()) {
		t.Errorf("Formats len = %d, want %d", got, len(knownFormats()))
	}
}

func TestRegistry_FormatsSorted(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	formats := reg.Formats()
	for i := 1; i < len(formats); i++ {
		if formats[i-1].Format > formats[i].Format {
			t.Fatalf("Formats not sorted at %d: %q > %q", i, formats[i-1].Format, formats[i].Format)
		}
	}
}

func TestRegistry_SupportedFormatsSorted(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	fs := reg.SupportedFormats()
	if len(fs) != len(knownFormats()) {
		t.Fatalf("SupportedFormats len = %d, want %d", len(fs), len(knownFormats()))
	}
	for i := 1; i < len(fs); i++ {
		if fs[i-1] > fs[i] {
			t.Fatalf("SupportedFormats not sorted at %d", i)
		}
	}
}

func TestWithFormat_OverridesEntry(t *testing.T) {
	t.Parallel()
	custom := FormatInfo{Format: FormatMP4, Type: Video, Extension: "mp4", MimeType: "video/custom"}
	reg := NewRegistry(WithFormat(custom))
	fi, ok := reg.Lookup(FormatMP4)
	if !ok || fi.MimeType != "video/custom" {
		t.Errorf("WithFormat did not override entry: %+v ok=%v", fi, ok)
	}
}

func TestWithProber_NilIsIgnored(t *testing.T) {
	t.Parallel()
	var typedNil *imageProber // typed-nil interface value
	reg := NewRegistry(WithProber(nil), WithProber(typedNil))
	// No panic and probe still works with signature-only detection.
	meta := reg.Probe(encodePNG(t, 4, 4))
	if meta.Type != Image {
		t.Errorf("expected Image, got %v", meta.Type)
	}
	if !meta.Resolution.IsZero() {
		t.Errorf("expected no dimensions without image prober, got %v", meta.Resolution)
	}
}

func TestRegistry_ProbeEnrichesImageDimensions(t *testing.T) {
	t.Parallel()
	reg := NewRegistry(WithImageProber())
	meta := reg.Probe(encodePNG(t, 24, 12))
	if meta.Type != Image || meta.Format != FormatPNG {
		t.Errorf("classification = %+v, want png image", meta.Info)
	}
	if meta.Resolution.Width != 24 || meta.Resolution.Height != 12 {
		t.Errorf("dimensions = %v, want 24x12", meta.Resolution)
	}
}

func TestRegistry_ProbeFallsBackToDetectionForNonImage(t *testing.T) {
	t.Parallel()
	reg := NewRegistry(WithImageProber())
	wav := make([]byte, 12)
	copy(wav[0:4], "RIFF")
	copy(wav[8:12], "WAVE")
	meta := reg.Probe(wav)
	if meta.Type != Audio || meta.Format != FormatWAV {
		t.Errorf("classification = %+v, want wav audio", meta.Info)
	}
	if !meta.Resolution.IsZero() {
		t.Errorf("non-image should have no dimensions, got %v", meta.Resolution)
	}
}

func TestRegistry_ProbeUsesFirstMatchingProber(t *testing.T) {
	t.Parallel()
	sentinel := &countingProber{meta: Metadata{Resolution: Resolution{Width: 99, Height: 88}}}
	reg := NewRegistry(WithProber(sentinel), WithImageProber())
	meta := reg.Probe(encodePNG(t, 10, 10))
	if meta.Resolution.Width != 99 || meta.Resolution.Height != 88 {
		t.Errorf("expected first prober to win, got %v", meta.Resolution)
	}
	if sentinel.calls != 1 {
		t.Errorf("expected sentinel called once, got %d", sentinel.calls)
	}
}

// countingProber always succeeds and records how many times it was called.
type countingProber struct {
	meta  Metadata
	calls int
}

func (c *countingProber) Probe(_ []byte) (Metadata, error) {
	c.calls++
	return c.meta, nil
}

func TestImageProber_Probe(t *testing.T) {
	t.Parallel()
	meta, err := imageProber{}.Probe(encodeJPEG(t, 32, 16))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta.Type != Image || meta.Resolution.Width != 32 || meta.Resolution.Height != 16 {
		t.Errorf("meta = %+v, want 32x16 image", meta)
	}
}

func TestImageProber_RejectsNonImage(t *testing.T) {
	t.Parallel()
	if _, err := (imageProber{}).Probe([]byte("plain text, not an image")); err == nil {
		t.Fatal("expected error for non-image content")
	}
}
