package render

import (
	"fmt"
	"io"

	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

// StatusReporter emits one-off status feedback lines for guided,
// multi-step CLI flows over an injected writer, palette, and glyph set.
//
// Where progress animates ongoing work and prompt reads input, this renders the short,
// one-shot status lines a flow emits between steps: a green "✓ Detected Go",
// a "[1/4]" step counter, a section heading, or a warn/error notice.
// It composes a [theme.Palette] for color and a [theme.Glyphs] set for the leading symbol,
// so every line honors NO_COLOR, TTY detection, and UTF-8 capability.
type StatusReporter struct {
	writer  io.Writer
	palette theme.Palette
	glyphs  theme.Glyphs
}

// NewStatusReporter builds a reporter from an explicit writer, palette, and glyph set.
// Callers bind the writer to a real stream (typically stderr, the "diagnostics to stderr" convention) while tests pass an in-memory buffer.
func NewStatusReporter(w io.Writer, palette theme.Palette, glyphs theme.Glyphs) *StatusReporter {
	return &StatusReporter{writer: w, palette: palette, glyphs: glyphs}
}

// Success emits a success line: a green check glyph followed by message.
func (r *StatusReporter) Success(message string) error {
	return r.writeLine(r.palette.Success(r.glyphs.Success()), message)
}

// Error emits an error line: a red cross glyph followed by message.
func (r *StatusReporter) Error(message string) error {
	return r.writeLine(r.palette.Error(r.glyphs.Error()), message)
}

// Warn emits a warning line: a yellow warning glyph followed by message.
func (r *StatusReporter) Warn(message string) error {
	return r.writeLine(r.palette.Warn(r.glyphs.Warning()), message)
}

// Info emits an informational line: a cyan info glyph followed by message.
func (r *StatusReporter) Info(message string) error {
	return r.writeLine(r.palette.Info(r.glyphs.Info()), message)
}

// Bullet emits an indented bullet line: a dimmed bullet glyph followed by message.
func (r *StatusReporter) Bullet(message string) error {
	return r.writeLine("  "+r.palette.Dim(r.glyphs.Bullet()), message)
}

// Step emits a step line prefixed with a dimmed "[current/total]" counter.
func (r *StatusReporter) Step(current, total int, message string) error {
	counter := r.palette.Dim(fmt.Sprintf("[%d/%d]", current, total))
	return r.writeLine(counter, message)
}

// Heading emits a bold section heading preceded by a blank line.
func (r *StatusReporter) Heading(title string) error {
	if _, err := fmt.Fprintf(r.writer, "\n%s\n", r.palette.Bold(title)); err != nil {
		return errors.Internal(err)
	}
	return nil
}

func (r *StatusReporter) writeLine(prefix, message string) error {
	if _, err := fmt.Fprintf(r.writer, "%s %s\n", prefix, message); err != nil {
		return errors.Internal(err)
	}
	return nil
}
