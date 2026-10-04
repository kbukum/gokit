package util

import "time"

// Timer is a clock-owned one-shot timer. The owner must stop timers it no longer needs.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// TimerClock extends a Clock with injectable scheduling.
type TimerClock interface {
	Clock
	NewTimer(time.Duration) Timer
}

// MonotonicClock retains time.Now's monotonic reading for elapsed-time leases.
// It is independent of UTC/domain clocks; standard timers remain advancing when wall time changes.
type MonotonicClock struct{}

// Now returns wall time together with its monotonic reading.
func (MonotonicClock) Now() time.Time { return time.Now() }

// NewTimer schedules an independently advancing runtime timer.
func (MonotonicClock) NewTimer(d time.Duration) Timer { return SystemClock{}.NewTimer(d) }

type systemTimer struct{ timer *time.Timer }

func (t systemTimer) C() <-chan time.Time { return t.timer.C }
func (t systemTimer) Stop() bool          { return t.timer.Stop() }

// NewTimer schedules a wall-clock timer.
func (SystemClock) NewTimer(d time.Duration) Timer { return systemTimer{timer: time.NewTimer(d)} }

type fakeTimer struct {
	clock *FakeClock
	ch    chan time.Time
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	_, ok := t.clock.timers[t]
	delete(t.clock.timers, t)
	return ok
}

// NewTimer schedules against fake time; Advance and Set synchronously deliver due timers.
func (c *FakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, ch: make(chan time.Time, 1)}
	c.timers[t] = c.now.Add(d)
	c.fireTimers()
	return t
}

func (c *FakeClock) fireTimers() {
	for timer, deadline := range c.timers {
		if !deadline.After(c.now) {
			timer.ch <- deadline
			delete(c.timers, timer)
		}
	}
}
