package theme_test

import (
	"os"
	"strings"
	"testing"

	"github.com/kbukum/gokit/cli/theme"
)

func TestResolveColorNoColorOverridesAlways(t *testing.T) {
	t.Parallel()
	if theme.ResolveColorWith(theme.ColorAlways, true, true) {
		t.Error("NO_COLOR must override ColorAlways")
	}
	if theme.ResolveColorWith(theme.ColorAlways, true, false) {
		t.Error("NO_COLOR must override ColorAlways regardless of TTY")
	}
}

func TestResolveColorAbsoluteChoices(t *testing.T) {
	t.Parallel()
	if !theme.ResolveColorWith(theme.ColorAlways, false, false) {
		t.Error("ColorAlways without NO_COLOR must be on")
	}
	if theme.ResolveColorWith(theme.ColorNever, false, true) {
		t.Error("ColorNever must be off even on a TTY")
	}
}

func TestResolveColorAutoFollowsTerminal(t *testing.T) {
	t.Parallel()
	if !theme.ResolveColorWith(theme.ColorAuto, false, true) {
		t.Error("ColorAuto must follow the terminal (on)")
	}
	if theme.ResolveColorWith(theme.ColorAuto, false, false) {
		t.Error("ColorAuto must follow the terminal (off)")
	}
}

func TestColorChoiceRoundTripsByName(t *testing.T) {
	t.Parallel()
	for _, choice := range []theme.ColorChoice{theme.ColorAuto, theme.ColorAlways, theme.ColorNever} {
		got, ok := theme.ParseColorChoice(choice.String())
		if !ok || got != choice {
			t.Errorf("round trip failed for %v: got %v ok=%v", choice, got, ok)
		}
	}
	if _, ok := theme.ParseColorChoice("bogus"); ok {
		t.Error("bogus choice must not parse")
	}
}

func TestDisabledPaletteIsIdentity(t *testing.T) {
	t.Parallel()
	plain := theme.NewPalette(false)
	if plain.Success("ok") != "ok" || plain.Error("boom") != "boom" {
		t.Error("disabled palette must be the identity function")
	}
	if plain.Enabled() {
		t.Error("palette must report disabled")
	}
}

func TestEnabledPaletteWrapsInSGRAndResets(t *testing.T) {
	t.Parallel()
	color := theme.NewPalette(true)
	if got := color.Success("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("success = %q", got)
	}
	if got := color.Error("boom"); got != "\x1b[31mboom\x1b[0m" {
		t.Errorf("error = %q", got)
	}
	if !color.Enabled() {
		t.Error("palette must report enabled")
	}
}

func TestResolveColorReadsEnv(t *testing.T) {
	t.Setenv(theme.NoColorEnv, "")
	// NO_COLOR present (even empty) disables color for any auto/always request.
	if theme.ResolveColor(theme.ColorAlways, true) {
		t.Error("NO_COLOR present must disable color via the env-aware resolver")
	}
	if !theme.NoColorEnvSet() {
		t.Error("NoColorEnvSet must report presence of an empty NO_COLOR")
	}
}

func TestPaletteForStreamResolvesAgainstTerminalFlag(t *testing.T) {
	t.Parallel()
	if theme.PaletteForStream(theme.ColorNever, true).Enabled() {
		t.Error("ColorNever must keep the palette disabled")
	}
	if !theme.PaletteForStream(theme.ColorAlways, false).Enabled() {
		t.Error("ColorAlways must enable the palette")
	}
}

func TestGlyphsUnicodeSetEmitsSymbols(t *testing.T) {
	t.Parallel()
	g := theme.NewGlyphs(true)
	if !g.Unicode() {
		t.Fatal("glyphs must report unicode")
	}
	cases := map[string]string{
		g.Success(): "✓", g.Error(): "✗", g.Warning(): "⚠", g.Info(): "ℹ",
		g.Bullet(): "•", g.Arrow(): "→", g.Pointer(): "❯", g.Answer(): "»",
		g.RadioOn(): "◉", g.RadioOff(): "○", g.ArrowUp(): "↑", g.ArrowDown(): "↓",
		g.Ellipsis(): "…",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("glyph = %q, want %q", got, want)
		}
	}
}

func TestGlyphsASCIIFallbackIsByteClean(t *testing.T) {
	t.Parallel()
	g := theme.NewGlyphs(false)
	if g.Unicode() {
		t.Fatal("glyphs must report ASCII")
	}
	for _, symbol := range []string{
		g.Success(), g.Error(), g.Warning(), g.Info(), g.Bullet(), g.Arrow(),
		g.Pointer(), g.Answer(), g.RadioOn(), g.RadioOff(), g.ArrowUp(),
		g.ArrowDown(), g.Ellipsis(),
	} {
		for _, r := range symbol {
			if r > 127 {
				t.Errorf("fallback must be ASCII: %q", symbol)
			}
		}
	}
}

func TestUnicodeEnvEnabledDetectsUTF8(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "en_US.UTF-8")
	if !theme.UnicodeEnvEnabled() {
		t.Error("UTF-8 LANG must enable unicode")
	}
	if !theme.GlyphsFromEnv().Unicode() {
		t.Error("GlyphsFromEnv must mirror UnicodeEnvEnabled")
	}
}

func TestUnicodeEnvDisabledForNonUTF8Locale(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "POSIX")
	t.Setenv("LANG", "en_US.ISO-8859-1")
	if theme.UnicodeEnvEnabled() {
		t.Error("non-UTF-8 locale must not enable unicode")
	}
}

func TestColorChoiceStringForUnknown(t *testing.T) {
	t.Parallel()
	if got := theme.ColorChoice(99).String(); !strings.Contains(got, "99") {
		t.Errorf("unknown choice string = %q", got)
	}
}

func TestEnabledPaletteSecondaryColors(t *testing.T) {
	t.Parallel()
	color := theme.NewPalette(true)
	cases := map[string]string{
		color.Warn("w"): "\x1b[33mw\x1b[0m",
		color.Info("i"): "\x1b[36mi\x1b[0m",
		color.Dim("d"):  "\x1b[2md\x1b[0m",
		color.Bold("b"): "\x1b[1mb\x1b[0m",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("paint = %q, want %q", got, want)
		}
	}
}

func TestDisabledPaletteSecondaryColorsAreIdentity(t *testing.T) {
	t.Parallel()
	plain := theme.NewPalette(false)
	if plain.Warn("w") != "w" || plain.Info("i") != "i" || plain.Dim("d") != "d" || plain.Bold("b") != "b" {
		t.Error("disabled palette must not paint secondary colors")
	}
}

func TestUnicodeEnvEnabledAcceptsDashlessUTF8(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "ja_JP.utf8")
	if !theme.UnicodeEnvEnabled() {
		t.Error("a locale naming utf8 (no dash) must enable unicode")
	}
}

func TestUnicodeEnvSkipsUnsetKeysBeforeMatch(t *testing.T) {
	t.Setenv("LC_ALL", "")
	os.Unsetenv("LC_ALL")
	t.Setenv("LC_CTYPE", "")
	os.Unsetenv("LC_CTYPE")
	t.Setenv("LANG", "en_US.UTF-8")
	if !theme.UnicodeEnvEnabled() {
		t.Error("must skip unset locale keys and honor the first set UTF-8 key")
	}
}
