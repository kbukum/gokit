package theme

import (
	"fmt"
	"os"
)

// NoColorEnv is the environment variable that, when present (regardless of value),
// disables color regardless of any explicit choice. See the NO_COLOR standard:
// https://no-color.org.
const NoColorEnv = "NO_COLOR"

// ColorChoice is a user's requested color policy, before environment/TTY resolution.
type ColorChoice int

const (
	// ColorAuto enables color only when writing to a terminal and NO_COLOR is unset.
	// It is the zero value, so an unset choice defaults to auto.
	ColorAuto ColorChoice = iota
	// ColorAlways forces color on (still overridden by NO_COLOR).
	ColorAlways
	// ColorNever forces color off.
	ColorNever
)

// String returns the canonical lowercase name of the choice.
func (c ColorChoice) String() string {
	switch c {
	case ColorAuto:
		return "auto"
	case ColorAlways:
		return "always"
	case ColorNever:
		return "never"
	default:
		return fmt.Sprintf("ColorChoice(%d)", int(c))
	}
}

// ParseColorChoice parses a choice from its lowercase name (auto/always/never).
//
// The second return value is false for any other value,
// so the caller can raise its own typed usage error naming the accepted values.
func ParseColorChoice(name string) (ColorChoice, bool) {
	switch name {
	case "auto":
		return ColorAuto, true
	case "always":
		return ColorAlways, true
	case "never":
		return ColorNever, true
	default:
		return ColorAuto, false
	}
}

// NoColorEnvSet reports whether the NO_COLOR environment variable is present.
//
// Per the NO_COLOR standard (https://no-color.org) any presence disables color,
// so an empty value (NO_COLOR=) still counts.
func NoColorEnvSet() bool {
	_, ok := os.LookupEnv(NoColorEnv)
	return ok
}

// ResolveColor resolves a [ColorChoice] into an effective on/off decision.
//
// Resolution order (the NO_COLOR standard takes precedence over an explicit request):
// if NO_COLOR is set the result is off; otherwise ColorAlways is on, ColorNever is off,
// and ColorAuto follows isTerminal. It reads the process environment for NO_COLOR;
// use [ResolveColorWith] for a fully injected, environment-free decision.
func ResolveColor(choice ColorChoice, isTerminal bool) bool {
	return ResolveColorWith(choice, NoColorEnvSet(), isTerminal)
}

// ResolveColorWith is the pure resolver core: it folds an explicit NO_COLOR presence
// and TTY detection into an effective on/off decision, so it is environment-free and unit-testable.
//
// NO_COLOR wins over every choice; otherwise ColorAlways/ColorNever are absolute
// and ColorAuto follows isTerminal.
func ResolveColorWith(choice ColorChoice, noColor, isTerminal bool) bool {
	if noColor {
		return false
	}
	switch choice {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // ColorAuto
		return isTerminal
	}
}

// Palette is a resolved, semantic color palette.
//
// When disabled every style is the identity function,
// so callers render the same way regardless of terminal capability.
// Construct it from a resolved boolean via [NewPalette],
// or resolve a [ColorChoice] against a stream's TTY status via [PaletteForStream].
type Palette struct {
	enabled bool
}

// NewPalette returns a palette with color explicitly enabled or disabled.
func NewPalette(enabled bool) Palette {
	return Palette{enabled: enabled}
}

// PaletteForStream resolves a palette for a specific output stream by folding NO_COLOR, the choice,
// and the stream's TTY status via [ResolveColor].
func PaletteForStream(choice ColorChoice, isTerminal bool) Palette {
	return NewPalette(ResolveColor(choice, isTerminal))
}

// Enabled reports whether this palette emits color.
func (p Palette) Enabled() bool { return p.enabled }

// Success paints text green — successful/complete status.
func (p Palette) Success(text string) string { return p.paint("32", text) }

// Error paints text red — failure/error status.
func (p Palette) Error(text string) string { return p.paint("31", text) }

// Warn paints text yellow — warnings and attention.
func (p Palette) Warn(text string) string { return p.paint("33", text) }

// Info paints text cyan — informational/neutral highlight.
func (p Palette) Info(text string) string { return p.paint("36", text) }

// Dim paints text dimmed — secondary detail (cache/skip).
func (p Palette) Dim(text string) string { return p.paint("2", text) }

// Bold paints text bold — emphasis (headings, totals).
func (p Palette) Bold(text string) string { return p.paint("1", text) }

// paint wraps text in an SGR code plus reset when enabled, else returns it verbatim
// so callers on pipes or redirects stay byte-clean.
func (p Palette) paint(code, text string) string {
	if !p.enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
