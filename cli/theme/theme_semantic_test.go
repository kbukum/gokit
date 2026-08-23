package theme_test

import (
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/theme"
)

func TestThemeDisabledPaletteKeepsActionsAlignedAndEscapeFree(t *testing.T) {
	t.Parallel()
	out := theme.NewTheme(theme.NewPalette(false)).Action("Done", "pkg", theme.ToneSuccess)
	if want := "        Done pkg"; out != want {
		t.Errorf("action = %q, want %q", out, want)
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Error("disabled palette must not emit escape sequences")
	}
}

func TestThemeActionWidthIsConfigurable(t *testing.T) {
	t.Parallel()
	out := theme.NewTheme(theme.NewPalette(false)).WithActionWidth(8).Action("Done", "pkg", theme.ToneSuccess)
	if want := "    Done pkg"; out != want {
		t.Errorf("action = %q, want %q", out, want)
	}
}

func TestThemeOverlongLabelIsNotTruncatedAndGetsNoPadding(t *testing.T) {
	t.Parallel()
	out := theme.NewTheme(theme.NewPalette(false)).WithActionWidth(4).Action("Compiling", "pkg", theme.ToneInfo)
	if want := "Compiling pkg"; out != want {
		t.Errorf("action = %q, want %q", out, want)
	}
}

func TestThemeEnabledPaletteRendersCargoStyleActionsAndHeadings(t *testing.T) {
	t.Parallel()
	th := theme.NewTheme(theme.NewPalette(true))
	if got, want := th.Action("Checking", "pkg", theme.ToneInfo), "\x1b[1m\x1b[36m    Checking\x1b[0m\x1b[0m pkg"; got != want {
		t.Errorf("action = %q, want %q", got, want)
	}
	if got, want := th.Heading("Release plan"), "\x1b[1mRelease plan\x1b[0m"; got != want {
		t.Errorf("heading = %q, want %q", got, want)
	}
}

func TestThemeTonesMapToPaletteColors(t *testing.T) {
	t.Parallel()
	th := theme.NewTheme(theme.NewPalette(true)).WithActionWidth(0)
	cases := map[theme.Tone]string{
		theme.ToneSuccess: "\x1b[1m\x1b[32mx\x1b[0m\x1b[0m d",
		theme.ToneError:   "\x1b[1m\x1b[31mx\x1b[0m\x1b[0m d",
		theme.ToneWarning: "\x1b[1m\x1b[33mx\x1b[0m\x1b[0m d",
		theme.ToneInfo:    "\x1b[1m\x1b[36mx\x1b[0m\x1b[0m d",
		theme.ToneDim:     "\x1b[1m\x1b[2mx\x1b[0m\x1b[0m d",
	}
	for tone, want := range cases {
		if got := th.Action("x", "d", tone); got != want {
			t.Errorf("tone %v action = %q, want %q", tone, got, want)
		}
	}
}

func TestThemePaletteAccessor(t *testing.T) {
	t.Parallel()
	p := theme.NewPalette(true)
	if !theme.NewTheme(p).Palette().Enabled() {
		t.Error("Theme.Palette must return the resolved palette")
	}
}
