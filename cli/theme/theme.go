package theme

import (
	"strings"
	"unicode/utf8"
)

// Tone is the semantic outcome applied to an action label, selecting the
// [Palette] color a [Theme] paints the label with.
type Tone int

const (
	// ToneSuccess marks successful or completed work.
	ToneSuccess Tone = iota
	// ToneError marks failed work.
	ToneError
	// ToneWarning marks work requiring attention.
	ToneWarning
	// ToneInfo marks neutral progress or information.
	ToneInfo
	// ToneDim marks secondary or skipped work.
	ToneDim
)

// DefaultActionWidth is the default width, in terminal columns, of the
// right-aligned action-label column produced by [Theme.Action].
const DefaultActionWidth = 12

// Theme renders bold headings and right-aligned, Cargo-like action lines keyed
// by a semantic [Tone], on a resolved [Palette]. It is the semantic styling layer
// above the raw palette: callers describe intent (a heading, or a toned action)
// rather than picking colors.
//
// Labels are right-aligned by Unicode scalar count, which matches terminal columns
// for the ASCII/Latin action verbs it is built for ("Checking", "Compiling").
// Column-exact alignment of wide (CJK/emoji) labels is intentionally left to
// heavier kits.
type Theme struct {
	palette     Palette
	actionWidth int
}

// NewTheme builds a theme from a resolved palette with the default action width.
func NewTheme(palette Palette) Theme {
	return Theme{palette: palette, actionWidth: DefaultActionWidth}
}

// WithActionWidth returns a copy of the theme with the right-aligned action-label
// width (in columns) set; the receiver is unchanged.
func (t Theme) WithActionWidth(width int) Theme {
	t.actionWidth = width
	return t
}

// Palette returns the resolved palette the theme paints with.
func (t Theme) Palette() Palette { return t.palette }

// Heading renders a bold heading line.
func (t Theme) Heading(title string) string {
	return t.palette.Bold(title)
}

// Action renders a right-aligned, bold semantic action label followed by unstyled
// detail — the "  Checking pkg" form familiar from Cargo. The label is right-aligned
// to the configured action width; a label wider than that column is emitted whole
// without padding rather than truncated.
func (t Theme) Action(label, detail string, tone Tone) string {
	pad := t.actionWidth - utf8.RuneCountInString(label)
	if pad < 0 {
		pad = 0
	}
	aligned := strings.Repeat(" ", pad) + label
	var colored string
	switch tone {
	case ToneSuccess:
		colored = t.palette.Success(aligned)
	case ToneError:
		colored = t.palette.Error(aligned)
	case ToneWarning:
		colored = t.palette.Warn(aligned)
	case ToneInfo:
		colored = t.palette.Info(aligned)
	case ToneDim:
		colored = t.palette.Dim(aligned)
	default:
		colored = aligned
	}
	return t.palette.Bold(colored) + " " + detail
}
