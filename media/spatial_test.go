package media

import "testing"

func TestResolution_Presets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  Resolution
		want Resolution
	}{
		{"360p", Resolution360p(), Resolution{640, 360}},
		{"480p", Resolution480p(), Resolution{854, 480}},
		{"720p", Resolution720p(), Resolution{1280, 720}},
		{"1080p", Resolution1080p(), Resolution{1920, 1080}},
		{"1440p", Resolution1440p(), Resolution{2560, 1440}},
		{"4k", Resolution4K(), Resolution{3840, 2160}},
		{"new", NewResolution(100, 50), Resolution{100, 50}},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestResolution_AspectRatio(t *testing.T) {
	t.Parallel()
	w, h := Resolution1080p().AspectRatio()
	if w != 16 || h != 9 {
		t.Errorf("AspectRatio = %d:%d, want 16:9", w, h)
	}
	if zw, zh := (Resolution{0, 0}).AspectRatio(); zw != 0 || zh != 0 {
		t.Errorf("zero AspectRatio = %d:%d, want 0:0", zw, zh)
	}
	if got := Resolution1080p().AspectRatioFloat(); got < 1.77 || got > 1.78 {
		t.Errorf("AspectRatioFloat = %v, want ~1.777", got)
	}
	if got := (Resolution{10, 0}).AspectRatioFloat(); got != 0 {
		t.Errorf("zero-height AspectRatioFloat = %v, want 0", got)
	}
}

func TestResolution_Orientation(t *testing.T) {
	t.Parallel()
	if !(Resolution{9, 16}).IsPortrait() {
		t.Error("expected portrait")
	}
	if !(Resolution{16, 9}).IsLandscape() {
		t.Error("expected landscape")
	}
	if !(Resolution{50, 50}).IsSquare() {
		t.Error("expected square")
	}
	if !(Resolution{0, 5}).IsZero() || (Resolution{5, 5}).IsZero() {
		t.Error("IsZero mismatch")
	}
}

func TestResolution_PixelCountAndString(t *testing.T) {
	t.Parallel()
	if got := Resolution1080p().PixelCount(); got != 2_073_600 {
		t.Errorf("PixelCount = %d, want 2073600", got)
	}
	if got := (Resolution{1920, 1080}).String(); got != "1920x1080" {
		t.Errorf("String = %q", got)
	}
}

func TestResolution_Scaling(t *testing.T) {
	t.Parallel()
	r := Resolution{1920, 1080}
	if got := r.ScaleToFit(1280, 1280); got != (Resolution{1280, 720}) {
		t.Errorf("ScaleToFit = %v, want 1280x720", got)
	}
	if got := r.ScaleToFill(1280, 1280); got != (Resolution{2276, 1280}) {
		t.Errorf("ScaleToFill = %v, want 2276x1280", got)
	}
	if got := r.ScaleBy(0.5); got != (Resolution{960, 540}) {
		t.Errorf("ScaleBy = %v, want 960x540", got)
	}
	if got := (Resolution{0, 0}).ScaleToFit(100, 100); got != (Resolution{0, 0}) {
		t.Errorf("scaling zero resolution = %v, want 0x0", got)
	}
}

func TestFrameRate_PresetsAndFloat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  FrameRate
		want FrameRate
		fps  float64
	}{
		{"fps", FPS(48), FrameRate{48, 1}, 48},
		{"new", NewFrameRate(48, 2), FrameRate{48, 2}, 24},
		{"24", FPS24(), FrameRate{24, 1}, 24},
		{"25", FPS25(), FrameRate{25, 1}, 25},
		{"30", FPS30(), FrameRate{30, 1}, 30},
		{"50", FPS50(), FrameRate{50, 1}, 50},
		{"60", FPS60(), FrameRate{60, 1}, 60},
		{"ntsc24", NTSC24(), FrameRate{24000, 1001}, 23.976023976023978},
		{"ntsc30", NTSC30(), FrameRate{30000, 1001}, 29.97002997002997},
		{"ntsc60", NTSC60(), FrameRate{60000, 1001}, 59.94005994005994},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
		if tt.got.Float() != tt.fps {
			t.Errorf("%s Float = %v, want %v", tt.name, tt.got.Float(), tt.fps)
		}
	}
	if got := (FrameRate{30, 0}).Float(); got != 0 {
		t.Errorf("zero-denominator Float = %v, want 0", got)
	}
}
