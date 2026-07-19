package live

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kbukum/gokit/cli/theme"
	"github.com/kbukum/gokit/errors"
)

const (
	// defaultMaxRegions bounds how many concurrent tiles a console shows.
	defaultMaxRegions = 8
	// defaultRegionHeight bounds how many recent lines each tile retains.
	defaultRegionHeight = 5
)

// Config bounds a [Console]'s live area.
// The zero value is completed by [NewConsole] with the package defaults.
type Config struct {
	// MaxRegions caps the number of concurrent regions;
	// [Console.AddRegion] rejects further regions once reached (documented backpressure).
	MaxRegions int
	// RegionHeight caps the recent lines each region retains in the live view;
	// older lines scroll out of the ephemeral peek (documented per-region bound).
	RegionHeight int
}

func (c Config) withDefaults() Config {
	if c.MaxRegions <= 0 {
		c.MaxRegions = defaultMaxRegions
	}
	if c.RegionHeight <= 0 {
		c.RegionHeight = defaultRegionHeight
	}
	return c
}

// Console is a bounded multi-region live console over an injected writer.
//
// It is safe for concurrent use:
// regions may be appended to from separate goroutines while the owner periodically calls [Console.Render].
// Ownership, cancellation, and render cadence belong to the caller —
// the console itself spawns no goroutines.
type Console struct {
	writer io.Writer
	style  theme.Style
	config Config

	mu        sync.Mutex
	regions   []*Region
	lastFrame int
}

// NewConsole creates a live console writing to w, bounded by config.
// The [theme.Style] carries the palette and glyph set that select the verdict symbols,
// so [Console.Finish] stays byte-clean on non-UTF-8 terminals.
func NewConsole(w io.Writer, config Config, style theme.Style) *Console {
	return &Console{writer: w, style: style, config: config.withDefaults()}
}

// AddRegion appends a new tile titled title.
//
// It returns an error once the configured [Config.MaxRegions] bound is reached,
// so callers apply backpressure rather than growing the live area without limit.
func (c *Console) AddRegion(title string) (*Region, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.regions) >= c.config.MaxRegions {
		return nil, errors.InvalidInput("live", fmt.Sprintf("region limit reached (max %d)", c.config.MaxRegions))
	}
	region := &Region{title: title, height: c.config.RegionHeight}
	c.regions = append(c.regions, region)
	return region, nil
}

// Render redraws the whole live area in place: it moves the cursor up over the previous frame,
// clears it, and writes the current tiles.
func (c *Console) Render() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var b strings.Builder
	if c.lastFrame > 0 {
		fmt.Fprintf(&b, "\x1b[%dA\x1b[0J", c.lastFrame)
	}
	lines := 0
	for _, region := range c.regions {
		b.WriteString(c.style.Palette().Bold(region.title))
		b.WriteByte('\n')
		lines++
		for _, line := range region.snapshot() {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
			lines++
		}
	}
	c.lastFrame = lines
	if _, err := io.WriteString(c.writer, b.String()); err != nil {
		return errors.Internal(err)
	}
	return nil
}

// Finish clears the live area and writes each region's durable verdict to scrollback,
// so the terminal retains signal rather than transient progress.
func (c *Console) Finish() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var b strings.Builder
	if c.lastFrame > 0 {
		fmt.Fprintf(&b, "\x1b[%dA\x1b[0J", c.lastFrame)
	}
	c.lastFrame = 0
	for _, region := range c.regions {
		b.WriteString(region.verdictLine(c.style))
		b.WriteByte('\n')
	}
	if _, err := io.WriteString(c.writer, b.String()); err != nil {
		return errors.Internal(err)
	}
	return nil
}
