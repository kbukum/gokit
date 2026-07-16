package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/render"
	"github.com/kbukum/gokit/cli/theme"
)

func reporter(buf *bytes.Buffer, color, unicode bool) *render.StatusReporter {
	return render.NewStatusReporter(buf, theme.NewPalette(color), theme.NewGlyphs(unicode))
}

func TestStatusSuccessCarriesGlyphAndMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, true).Success("Detected Go"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓") || !strings.Contains(out, "Detected Go") {
		t.Errorf("success line = %q", out)
	}
}

func TestStatusStepRendersCounter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, true).Step(1, 4, "Selecting ecosystems"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "[1/4]") || !strings.Contains(out, "Selecting ecosystems") {
		t.Errorf("step line = %q", out)
	}
}

func TestStatusHeadingPrecededByBlankLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, true).Heading("Configuration"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "\n") || !strings.Contains(out, "Configuration") {
		t.Errorf("heading = %q", out)
	}
}

func TestStatusASCIIFallbackAvoidsUnicode(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, false).Warn("no toolchain found"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "!") || strings.Contains(out, "⚠") {
		t.Errorf("ascii fallback = %q", out)
	}
}

func TestStatusDisabledPaletteIsByteClean(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, true).Info("nothing to do"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("no-color must be byte-clean: %q", buf.String())
	}
}

func TestStatusEnabledPaletteEmitsEscapes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, true, true).Error("build failed"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b") {
		t.Errorf("color must emit SGR escapes: %q", buf.String())
	}
}

func TestStatusBulletIsIndented(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := reporter(&buf, false, true).Bullet("cached module"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "  ") || !strings.Contains(out, "•") || !strings.Contains(out, "cached module") {
		t.Errorf("bullet line = %q", out)
	}
}

func TestStatusWriteErrorSurfaces(t *testing.T) {
	t.Parallel()
	r := render.NewStatusReporter(failWriter{}, theme.NewPalette(false), theme.NewGlyphs(false))
	if err := r.Success("x"); err == nil {
		t.Error("write failure must surface")
	}
	if err := r.Heading("x"); err == nil {
		t.Error("heading write failure must surface")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errWrite }

type errString string

func (e errString) Error() string { return string(e) }

const errWrite errString = "write failed"
