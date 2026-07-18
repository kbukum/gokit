package progress

import (
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

// defaultBarWidth is the character width of the filled/empty gauge.
const defaultBarWidth = 20

// Bar is a determinate progress bar over an injected writer.
//
// Each render overwrites the current terminal line (a leading carriage return), so successive updates animate in place; [Bar.Finish] moves to a fresh line. The bar only advances when the caller moves its position, so it is fully deterministic without a clock.
type Bar struct {
	writer  io.Writer
	palette theme.Palette
	prefix  string
	width   int
	total   int64
	pos     int64
}

// BarOption configures a [Bar].
type BarOption func(*Bar)

// WithBarPrefix sets a label rendered before the gauge.
func WithBarPrefix(prefix string) BarOption {
	return func(b *Bar) { b.prefix = prefix }
}

// WithBarWidth sets the character width of the gauge (values below 1 are ignored).
func WithBarWidth(width int) BarOption {
	return func(b *Bar) {
		if width > 0 {
			b.width = width
		}
	}
}

// WithBarPalette styles the filled gauge and percentage with a palette.
func WithBarPalette(palette theme.Palette) BarOption {
	return func(b *Bar) { b.palette = palette }
}

// NewBar creates a bar for total units of work writing to w. A non-positive total renders as an immediately complete bar.
func NewBar(w io.Writer, total int64, opts ...BarOption) *Bar {
	if total < 0 {
		total = 0
	}
	b := &Bar{writer: w, palette: theme.NewPalette(false), width: defaultBarWidth, total: total}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// SetPosition sets the current position, clamped to [0, total], and renders.
func (b *Bar) SetPosition(pos int64) error {
	b.pos = clamp(pos, 0, b.total)
	return b.render()
}

// Inc advances the position by delta (clamped) and renders.
func (b *Bar) Inc(delta int64) error {
	return b.SetPosition(b.pos + delta)
}

// Finish fills the bar and moves to a fresh line.
func (b *Bar) Finish() error {
	b.pos = b.total
	if err := b.render(); err != nil {
		return err
	}
	if _, err := io.WriteString(b.writer, "\n"); err != nil {
		return errors.Internal(err)
	}
	return nil
}

// Percent returns the completion percentage in [0, 100].
func (b *Bar) Percent() int {
	if b.total <= 0 || b.pos >= b.total {
		return 100
	}
	// Integer math is exact for realistic totals; fall back to float64 (percent is coarse) only when pos*100 would overflow int64.
	if b.pos <= math.MaxInt64/100 {
		return int(b.pos * 100 / b.total)
	}
	return int(float64(b.pos) / float64(b.total) * 100)
}

func (b *Bar) render() error {
	pct := b.Percent()
	filled := b.width * pct / 100
	gauge := b.palette.Success(strings.Repeat("#", filled)) + strings.Repeat("-", b.width-filled)
	line := fmt.Sprintf("\r%s[%s] %d/%d %s",
		prefixSpace(b.prefix), gauge, b.pos, b.total, b.palette.Bold(fmt.Sprintf("%d%%", pct)))
	if _, err := io.WriteString(b.writer, line); err != nil {
		return errors.Internal(err)
	}
	return nil
}

func prefixSpace(prefix string) string {
	if prefix == "" {
		return ""
	}
	return prefix + " "
}

func clamp(v, lo, hi int64) int64 {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
