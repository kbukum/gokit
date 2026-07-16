package progress_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/progress"
	"github.com/kbukum/gokit/cli/theme"
)

func TestBarRendersPositionAndPercent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 10, progress.WithBarWidth(10), progress.WithBarPrefix("build"))
	if err := bar.SetPosition(4); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "build") {
		t.Errorf("bar missing prefix: %q", out)
	}
	if !strings.Contains(out, "4/10") || !strings.Contains(out, "40%") {
		t.Errorf("bar counters wrong: %q", out)
	}
	if !strings.HasPrefix(out, "\r") {
		t.Errorf("bar must lead with carriage return: %q", out)
	}
	if !strings.Contains(out, "####------") {
		t.Errorf("bar gauge wrong: %q", out)
	}
}

func TestBarIncAccumulates(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 4, progress.WithBarWidth(4))
	for range 4 {
		if err := bar.Inc(1); err != nil {
			t.Fatal(err)
		}
	}
	if bar.Percent() != 100 {
		t.Errorf("percent = %d, want 100", bar.Percent())
	}
}

func TestBarClampsOutOfRange(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 10)
	if err := bar.SetPosition(-5); err != nil {
		t.Fatal(err)
	}
	if bar.Percent() != 0 {
		t.Errorf("negative clamps to 0, got %d", bar.Percent())
	}
	if err := bar.SetPosition(100); err != nil {
		t.Fatal(err)
	}
	if bar.Percent() != 100 {
		t.Errorf("over-total clamps to 100, got %d", bar.Percent())
	}
}

func TestBarZeroTotalIsComplete(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 0)
	if bar.Percent() != 100 {
		t.Errorf("zero-total bar percent = %d, want 100", bar.Percent())
	}
}

func TestBarFinishEndsOnFreshLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 10)
	if err := bar.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("finish must end with newline: %q", buf.String())
	}
}

func TestBarPaletteEmitsColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, 10, progress.WithBarPalette(theme.NewPalette(true)))
	if err := bar.SetPosition(5); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b") {
		t.Errorf("colored bar must emit escapes: %q", buf.String())
	}
}

func TestSpinnerTickAdvancesFramesDeterministically(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spin := progress.NewSpinner(&buf, progress.WithSpinnerMessage("working"))
	for range 5 {
		if err := spin.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	out := buf.String()
	// ASCII frames cycle |, /, -, \, then wrap to | again.
	frames := strings.Count(out, "\r")
	if frames != 5 {
		t.Errorf("expected 5 rendered frames, got %d: %q", frames, out)
	}
	if !strings.Contains(out, "working") {
		t.Errorf("spinner missing message: %q", out)
	}
	if strings.Contains(out, "⠋") {
		t.Errorf("ascii spinner must stay byte-clean: %q", out)
	}
}

func TestSpinnerUnicodeFrames(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spin := progress.NewSpinner(&buf, progress.WithSpinnerGlyphs(theme.NewGlyphs(true)))
	if err := spin.Tick(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "⠋") {
		t.Errorf("unicode spinner must use braille frames: %q", buf.String())
	}
}

func TestSpinnerFinishWritesSuccessGlyphAfterSetMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spin := progress.NewSpinner(&buf, progress.WithSpinnerGlyphs(theme.NewGlyphs(true)))
	spin.SetMessage("working")
	if err := spin.Tick(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "working") {
		t.Errorf("tick must render the set message: %q", buf.String())
	}
	if err := spin.Finish("done"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓") || !strings.Contains(out, "done") {
		t.Errorf("finish line = %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("finish must end on a fresh line: %q", out)
	}
}

func TestSpinnerPaletteEmitsColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spin := progress.NewSpinner(&buf, progress.WithSpinnerPalette(theme.NewPalette(true)))
	if err := spin.Tick(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\x1b") {
		t.Errorf("colored spinner must emit escapes: %q", buf.String())
	}
}

func TestSpinnerASCIIGlyphsUseFallbackFrames(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	spin := progress.NewSpinner(&buf, progress.WithSpinnerGlyphs(theme.NewGlyphs(false)))
	if err := spin.Tick(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "⠋") {
		t.Errorf("ascii glyph set must stay byte-clean: %q", buf.String())
	}
}

func TestSpinnerTickPropagatesWriteError(t *testing.T) {
	t.Parallel()
	spin := progress.NewSpinner(failWriter{})
	if err := spin.Tick(); err == nil {
		t.Error("tick must surface a write failure")
	}
}

func TestSpinnerFinishPropagatesWriteError(t *testing.T) {
	t.Parallel()
	spin := progress.NewSpinner(failWriter{})
	if err := spin.Finish("done"); err == nil {
		t.Error("finish must surface a write failure")
	}
}

func TestBarRenderPropagatesWriteError(t *testing.T) {
	t.Parallel()
	bar := progress.NewBar(failWriter{}, 10)
	if err := bar.SetPosition(5); err == nil {
		t.Error("render must surface a write failure")
	}
}

func TestBarFinishPropagatesWriteError(t *testing.T) {
	t.Parallel()
	bar := progress.NewBar(failWriter{}, 10)
	if err := bar.Finish(); err == nil {
		t.Error("finish must surface a write failure")
	}
}

func TestBarNegativeTotalRendersComplete(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	bar := progress.NewBar(&buf, -1)
	if err := bar.SetPosition(5); err != nil {
		t.Fatal(err)
	}
	if bar.Percent() != 100 {
		t.Errorf("negative-total bar percent = %d, want 100", bar.Percent())
	}
}

func TestBarFinishPropagatesNewlineWriteError(t *testing.T) {
	t.Parallel()
	// Succeed on the gauge render, fail on the trailing newline write.
	bar := progress.NewBar(&flakyWriter{failAfter: 1}, 10)
	if err := bar.Finish(); err == nil {
		t.Error("finish must surface the trailing-newline write failure")
	}
}

// failWriter always fails, so error propagation through the renderers is
// deterministic without a real terminal.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errString("write failed") }

// flakyWriter succeeds for the first failAfter writes, then fails, exercising
// error paths that only trigger on a later write.
type flakyWriter struct {
	failAfter int
	count     int
}

func (w *flakyWriter) Write(p []byte) (int, error) {
	w.count++
	if w.count > w.failAfter {
		return 0, errString("write failed")
	}
	return len(p), nil
}

type errString string

func (e errString) Error() string { return string(e) }
