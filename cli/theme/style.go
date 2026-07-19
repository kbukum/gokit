package theme

// Style bundles a color [Palette] and a [Glyphs] set — the two visual capabilities a terminal renderer resolves from NO_COLOR and UTF-8 support — into one value, so renderers thread a single style rather than a separate palette/glyphs pair.
type Style struct {
	palette Palette
	glyphs  Glyphs
}

// NewStyle bundles a palette and glyph set into a rendering style.
func NewStyle(palette Palette, glyphs Glyphs) Style {
	return Style{palette: palette, glyphs: glyphs}
}

// Palette returns the color palette.
func (s Style) Palette() Palette { return s.palette }

// Glyphs returns the glyph set.
func (s Style) Glyphs() Glyphs { return s.glyphs }
