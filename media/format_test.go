package media

import "testing"

func TestKnownFormats_ReturnsFreshCopy(t *testing.T) {
	t.Parallel()
	a := knownFormats()
	if len(a) == 0 {
		t.Fatal("knownFormats returned empty catalog")
	}
	a[0].Extension = "mutated"
	b := knownFormats()
	if b[0].Extension == "mutated" {
		t.Error("knownFormats leaks shared state between calls")
	}
}

func TestKnownFormats_EntriesAreConsistent(t *testing.T) {
	t.Parallel()
	seen := make(map[Format]bool)
	for _, fi := range knownFormats() {
		if fi.Format == FormatUnknown {
			t.Errorf("catalog entry has empty format: %+v", fi)
		}
		if seen[fi.Format] {
			t.Errorf("duplicate catalog entry for %q", fi.Format)
		}
		seen[fi.Format] = true
		if fi.Extension == "" || fi.MimeType == "" {
			t.Errorf("catalog entry %q missing extension or mime: %+v", fi.Format, fi)
		}
		if fi.Type == Unknown {
			t.Errorf("catalog entry %q has Unknown type", fi.Format)
		}
	}
}

func TestKnownFormats_CoversEveryDetectedFormat(t *testing.T) {
	t.Parallel()
	catalog := make(map[Format]bool)
	for _, fi := range knownFormats() {
		catalog[fi.Format] = true
	}
	// Every format a signature detector can emit must have a catalog entry so
	// Registry.Lookup never returns a hole for detected content.
	detected := []Format{
		FormatJPEG, FormatPNG, FormatGIF, FormatWebP, FormatBMP, FormatTIFF,
		FormatICO, FormatAVIF, FormatHEIF, FormatMP4, FormatMOV, FormatM4V,
		FormatWebM, FormatMKV, FormatAVI, FormatFLV, FormatTS, FormatWAV,
		FormatFLAC, FormatOGG, FormatAAC, FormatMP3, FormatMIDI, FormatAIFF,
		FormatM4A, FormatText,
	}
	for _, f := range detected {
		if !catalog[f] {
			t.Errorf("format %q is detectable but missing from the catalog", f)
		}
	}
}
