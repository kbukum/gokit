package live

import (
	"sync"

	"github.com/kbukum/gokit/cli/theme"
)

// regionStatus is a region's terminal verdict.
type regionStatus int

const (
	statusRunning regionStatus = iota
	statusDone
	statusFailed
)

// Region is a single bounded tile within a [Console].
//
// It retains only its most recent lines (up to the console's configured height);
// older lines scroll out of the live peek. A region is safe to append to from a
// goroutine separate from the one rendering the console.
type Region struct {
	title  string
	height int

	mu     sync.Mutex
	lines  []string
	status regionStatus
	reason string
}

// Println appends a line to the region, dropping the oldest retained line once
// the height bound is exceeded (the ephemeral-peek bound).
func (r *Region) Println(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	if len(r.lines) > r.height {
		r.lines = r.lines[len(r.lines)-r.height:]
	}
}

// Done marks the region complete with a short summary shown on
// [Console.Finish].
func (r *Region) Done(summary string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = statusDone
	r.reason = summary
}

// Fail marks the region failed with a reason shown on [Console.Finish].
func (r *Region) Fail(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = statusFailed
	r.reason = reason
}

// snapshot returns a copy of the currently retained live lines.
func (r *Region) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// verdictLine renders the region's durable one-line verdict for scrollback.
func (r *Region) verdictLine(palette theme.Palette, glyphs theme.Glyphs) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	label := r.title
	if r.reason != "" {
		label = r.title + ": " + r.reason
	}
	switch r.status {
	case statusDone:
		return palette.Success(glyphs.Success() + " " + label)
	case statusFailed:
		return palette.Error(glyphs.Error() + " " + label)
	default:
		return palette.Dim(glyphs.Ellipsis() + " " + label)
	}
}
