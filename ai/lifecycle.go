package ai

import (
	"sync"
	"time"
)

// Lifecycle is a small mixin for AI-plug types that natively implement component.Component (per locked decision D12).
// It is concurrency-safe and tracks ready state plus the last-call timestamp used by Health.
//
// Embed it as a value field (not a pointer) and call MarkReady from Start, MarkStopped from Stop,
// and Touch on every successful provider call (Execute / Stream / Embed / Invoke / etc.).
type Lifecycle struct {
	mu       sync.RWMutex
	ready    bool
	stopped  bool
	lastCall time.Time
}

// MarkReady transitions the lifecycle to ready (called from Start).
func (l *Lifecycle) MarkReady() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ready = true
	l.stopped = false
}

// MarkStopped transitions the lifecycle to stopped (called from Stop).
func (l *Lifecycle) MarkStopped() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ready = false
	l.stopped = true
}

// Touch records the timestamp of the latest successful call.
func (l *Lifecycle) Touch() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastCall = time.Now()
}

// Ready reports whether the plug is ready to serve traffic.
func (l *Lifecycle) Ready() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ready
}

// LastCall returns the timestamp of the most recent successful call,
// or the zero time if no call has been recorded.
func (l *Lifecycle) LastCall() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastCall
}
