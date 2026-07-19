package render

import (
	"fmt"
	"io"

	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

// StatusReporter emits one-off status feedback lines for guided,
// multi-step CLI flows over an injected writer and [theme.Style].
//
// Where progress animates ongoing work and prompt reads input, this renders the short,
// one-shot status lines a flow emits between steps: a green "✓ Detected Go",
// a "[1/4]" step counter, a section heading, or a warn/error notice.
// The [theme.Style] carries the color palette and glyph set,
// so every line honors NO_COLOR, TTY detection, and UTF-8 capability.
type StatusReporter struct {
	writer io.Writer
	style  theme.Style
}

// NewStatusReporter builds a reporter from an explicit writer and style.
// Callers bind the writer to a real stream (typically stderr, the "diagnostics to stderr" convention) while tests pass an in-memory buffer.
func NewStatusReporter(w io.Writer, style theme.Style) *StatusReporter {
	return &StatusReporter{writer: w, style: style}
}

// Success emits a success line: a green check glyph followed by message.
func (r *StatusReporter) Success(message string) error {
	return r.writeLine(r.style.Palette().Success(r.style.Glyphs().Success()), message)
}

// Error emits an error line: a red cross glyph followed by message.
func (r *StatusReporter) Error(message string) error {
	return r.writeLine(r.style.Palette().Error(r.style.Glyphs().Error()), message)
}

// Warn emits a warning line: a yellow warning glyph followed by message.
func (r *StatusReporter) Warn(message string) error {
	return r.writeLine(r.style.Palette().Warn(r.style.Glyphs().Warning()), message)
}

// Info emits an informational line: a cyan info glyph followed by message.
func (r *StatusReporter) Info(message string) error {
	return r.writeLine(r.style.Palette().Info(r.style.Glyphs().Info()), message)
}

// Bullet emits an indented bullet line: a dimmed bullet glyph followed by message.
func (r *StatusReporter) Bullet(message string) error {
	return r.writeLine("  "+r.style.Palette().Dim(r.style.Glyphs().Bullet()), message)
}

// Step emits a step line prefixed with a dimmed "[current/total]" counter.
func (r *StatusReporter) Step(current, total int, message string) error {
	counter := r.style.Palette().Dim(fmt.Sprintf("[%d/%d]", current, total))
	return r.writeLine(counter, message)
}

// Heading emits a bold section heading preceded by a blank line.
func (r *StatusReporter) Heading(title string) error {
	if _, err := fmt.Fprintf(r.writer, "\n%s\n", r.style.Palette().Bold(title)); err != nil {
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
