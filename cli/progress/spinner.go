package progress

import (
	"io"

	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

// asciiSpinnerFrames is the pipe-clean fallback frame sequence.
var asciiSpinnerFrames = []string{"|", "/", "-", "\\"}

// unicodeSpinnerFrames is the braille spinner used on UTF-8 terminals.
var unicodeSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner is an indeterminate progress indicator over an injected writer.
//
// It advances one frame per [Spinner.Tick], so animation is driven by the caller
// rather than a background timer — deterministic and clock-free. Each tick
// overwrites the current line (leading carriage return); [Spinner.Finish]
// replaces the spinner with a final glyph and message on a fresh line.
type Spinner struct {
	writer  io.Writer
	palette theme.Palette
	glyphs  theme.Glyphs
	frames  []string
	message string
	index   int
}

// SpinnerOption configures a [Spinner].
type SpinnerOption func(*Spinner)

// WithSpinnerMessage sets the message rendered beside the spinner frame.
func WithSpinnerMessage(message string) SpinnerOption {
	return func(s *Spinner) { s.message = message }
}

// WithSpinnerPalette styles the spinner frame with a palette.
func WithSpinnerPalette(palette theme.Palette) SpinnerOption {
	return func(s *Spinner) { s.palette = palette }
}

// WithSpinnerGlyphs selects Unicode braille frames when the glyph set supports
// Unicode, else the ASCII fallback, and sets the completion glyph.
func WithSpinnerGlyphs(glyphs theme.Glyphs) SpinnerOption {
	return func(s *Spinner) {
		s.glyphs = glyphs
		if glyphs.Unicode() {
			s.frames = unicodeSpinnerFrames
		} else {
			s.frames = asciiSpinnerFrames
		}
	}
}

// NewSpinner creates a spinner writing to w.
func NewSpinner(w io.Writer, opts ...SpinnerOption) *Spinner {
	s := &Spinner{
		writer:  w,
		palette: theme.NewPalette(false),
		glyphs:  theme.NewGlyphs(false),
		frames:  asciiSpinnerFrames,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetMessage updates the message shown beside the spinner.
func (s *Spinner) SetMessage(message string) { s.message = message }

// Tick advances to the next frame and renders it in place.
func (s *Spinner) Tick() error {
	frame := s.palette.Info(s.frames[s.index%len(s.frames)])
	s.index++
	return s.write("\r" + frame + " " + s.message)
}

// Finish clears the spinner and writes a success glyph plus final message on a
// fresh line.
func (s *Spinner) Finish(message string) error {
	return s.write("\r" + s.palette.Success(s.glyphs.Success()) + " " + message + "\n")
}

func (s *Spinner) write(text string) error {
	if _, err := io.WriteString(s.writer, text); err != nil {
		return errors.Internal(err)
	}
	return nil
}
