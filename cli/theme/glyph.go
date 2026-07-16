package theme

import (
	"os"
	"strings"
)

// UTF8LocaleEnvs are the locale environment variables consulted, in precedence
// order, to decide whether the terminal encoding is UTF-8.
var UTF8LocaleEnvs = [...]string{"LC_ALL", "LC_CTYPE", "LANG"}

// UnicodeEnvEnabled reports whether the process locale advertises a UTF-8
// encoding.
//
// It consults [UTF8LocaleEnvs] in order and reports true when any is set to a
// value naming UTF-8 (case-insensitive, "utf-8" or "utf8"). When none is set the
// result is false, so the ASCII fallback is the safe default.
func UnicodeEnvEnabled() bool {
	for _, key := range UTF8LocaleEnvs {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "utf-8") || strings.Contains(lower, "utf8") {
			return true
		}
	}
	return false
}

// Glyphs is a resolved set of semantic status glyphs.
//
// When Unicode is disabled every accessor returns its ASCII fallback, so the
// same rendering code stays byte-clean on terminals that cannot display the
// Unicode symbols. Construct it from a resolved boolean via [NewGlyphs], or from
// the process locale via [GlyphsFromEnv].
type Glyphs struct {
	unicode bool
}

// NewGlyphs returns a glyph set with Unicode explicitly enabled or disabled.
func NewGlyphs(unicode bool) Glyphs {
	return Glyphs{unicode: unicode}
}

// GlyphsFromEnv resolves a glyph set from the process locale via
// [UnicodeEnvEnabled].
func GlyphsFromEnv() Glyphs {
	return NewGlyphs(UnicodeEnvEnabled())
}

// Unicode reports whether this set emits Unicode glyphs.
func (g Glyphs) Unicode() bool { return g.unicode }

// Success returns the success/completed glyph — "✓" (ASCII "v").
func (g Glyphs) Success() string { return g.pick("✓", "v") }

// Error returns the failure/error glyph — "✗" (ASCII "x").
func (g Glyphs) Error() string { return g.pick("✗", "x") }

// Warning returns the warning/attention glyph — "⚠" (ASCII "!").
func (g Glyphs) Warning() string { return g.pick("⚠", "!") }

// Info returns the informational glyph — "ℹ" (ASCII "i").
func (g Glyphs) Info() string { return g.pick("ℹ", "i") }

// Bullet returns the list bullet glyph — "•" (ASCII "*").
func (g Glyphs) Bullet() string { return g.pick("•", "*") }

// Arrow returns the progression arrow glyph — "→" (ASCII "->").
func (g Glyphs) Arrow() string { return g.pick("→", "->") }

// Pointer returns the selection pointer glyph — "❯" (ASCII ">").
func (g Glyphs) Pointer() string { return g.pick("❯", ">") }

// Answer returns the inline answer/input marker — "»" (ASCII ">").
func (g Glyphs) Answer() string { return g.pick("»", ">") }

// RadioOn returns the selected radio option glyph — "◉" (ASCII "(*)").
func (g Glyphs) RadioOn() string { return g.pick("◉", "(*)") }

// RadioOff returns the unselected radio option glyph — "○" (ASCII "( )").
func (g Glyphs) RadioOff() string { return g.pick("○", "( )") }

// ArrowUp returns the upward navigation arrow glyph — "↑" (ASCII "^").
func (g Glyphs) ArrowUp() string { return g.pick("↑", "^") }

// ArrowDown returns the downward navigation arrow glyph — "↓" (ASCII "v").
func (g Glyphs) ArrowDown() string { return g.pick("↓", "v") }

// Ellipsis returns the truncation ellipsis glyph — "…" (ASCII "...").
func (g Glyphs) Ellipsis() string { return g.pick("…", "...") }

// pick chooses the Unicode glyph when enabled, else the ASCII fallback.
func (g Glyphs) pick(unicode, ascii string) string {
	if g.unicode {
		return unicode
	}
	return ascii
}
