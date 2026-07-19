package theme_test

import (
	"testing"

	"github.com/kbukum/gokit/cli/theme"
)

func TestNewStyleExposesPaletteAndGlyphs(t *testing.T) {
	t.Parallel()
	palette := theme.NewPalette(true)
	glyphs := theme.NewGlyphs(true)
	style := theme.NewStyle(palette, glyphs)
	if style.Palette() != palette {
		t.Error("Style.Palette must return the bundled palette")
	}
	if style.Glyphs() != glyphs {
		t.Error("Style.Glyphs must return the bundled glyph set")
	}
}
