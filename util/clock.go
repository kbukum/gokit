package util

import (
	"sync"
	"time"
)

// Clock abstracts time so that production code can use SystemClock while tests
// inject a FakeClock with a controllable instant.
type Clock interface {
	Now() time.Time
}

// SystemClock is a real clock backed by time.Now().UTC().
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FakeClock is a deterministic clock for tests. Advance or set the time
// manually to exercise time-dependent logic without real delays.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock creates a FakeClock starting at initial. If initial is the zero
// value, it defaults to 2024-01-01T00:00:00Z.
func NewFakeClock(initial time.Time) *FakeClock {
	if initial.IsZero() {
		initial = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return &FakeClock{now: initial}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set sets an absolute time.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
