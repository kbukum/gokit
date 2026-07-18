package prompt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/cli/theme"
)

// Style bundles the color [theme.Palette] and [theme.Glyphs] set a prompt uses to render,
// so every frame honors NO_COLOR and UTF-8 capability.
type Style struct {
	palette theme.Palette
	glyphs  theme.Glyphs
}

// NewStyle bundles a palette and glyph set into a rendering style.
func NewStyle(palette theme.Palette, glyphs theme.Glyphs) Style {
	return Style{palette: palette, glyphs: glyphs}
}

// Palette returns the color palette.
func (s Style) Palette() theme.Palette { return s.palette }

// Glyphs returns the glyph set.
func (s Style) Glyphs() theme.Glyphs { return s.glyphs }

// heading returns the bold heading line shown above a prompt's choices or input.
func heading(style Style, prompt string) string {
	return style.palette.Bold(prompt)
}

// decorate renders a choice label plus its annotation and the recommended marker.
func decorate(style Style, choice Choice) string {
	var b strings.Builder
	b.WriteString(choice.Label())
	if choice.Annotation() != "" {
		b.WriteByte(' ')
		b.WriteString(style.palette.Dim(choice.Annotation()))
	}
	if choice.IsRecommended() {
		b.WriteByte(' ')
		b.WriteString(style.palette.Info("(recommended)"))
	}
	return b.String()
}

// numberedRows builds the static, one-based choice list a line-driven terminal prints.
func numberedRows(style Style, choices []Choice) []string {
	rows := make([]string, len(choices))
	for i, choice := range choices {
		rows[i] = fmt.Sprintf("  %d) %s", i+1, decorate(style, choice))
	}
	return rows
}

// writeAnswer writes the inline answer marker ("  » [hint]: ") and flushes.
func writeAnswer(term Terminal, style Style, hint string) error {
	marker := style.glyphs.Answer()
	text := "  " + marker + " "
	if hint != "" {
		text = "  " + marker + " " + hint + ": "
	}
	if err := term.Write(text); err != nil {
		return err
	}
	return term.Flush()
}

// notice writes a warning notice line beneath a line prompt.
func notice(term Terminal, style Style, text string) error {
	return term.WriteLine("  " + style.palette.Warn(text))
}

// parseIndex parses a one-based choice number into a zero-based index within [0, length);
// the second return value is false when out of range or unparsable.
func parseIndex(input string, length int) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || n < 1 || n > length {
		return 0, false
	}
	return n - 1, true
}
