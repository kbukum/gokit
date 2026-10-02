package sse

import (
	"context"
	"io"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Subscription owns one bounded live queue and reads replay directly from the shared ring. Next has one consumer; Close and Fail may run concurrently.
type Subscription struct {
	bus         *Bus
	principal   string
	route       string
	boundary    string
	replayAfter uint64
	replayUntil uint64
	queue       []record
	head        int
	count       int
	control     Event
	terminal    bool
	closed      bool
	wake        chan struct{}
	stop        func() bool
}

// Connected returns immutable metadata captured atomically with admission.
func (s *Subscription) Connected() Connected {
	return Connected{Epoch: s.bus.epoch, Cursor: s.boundary}
}

// Next returns a priority control, a replay event, or a live event. Loss is signaled by reset followed by EOF, never skipped. Filtered global sequence gaps are valid.
func (s *Subscription) Next(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		s.bus.mu.Lock()
		event, ready, err := s.nextLocked()
		s.bus.mu.Unlock()
		if ready || err != nil {
			return event, err
		}
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case <-s.wake:
		}
	}
}

func (s *Subscription) nextLocked() (Event, bool, error) {
	if s.closed {
		return Event{}, false, io.EOF
	}
	if s.control.Name != "" {
		event := s.control
		s.control = Event{}
		return event, true, nil
	}
	if s.terminal {
		return Event{}, false, io.EOF
	}
	b := s.bus
	if s.replayAfter < s.replayUntil {
		oldest := b.replay[b.head].sequence
		if s.replayAfter < oldest-1 {
			s.reset("replayExpired")
			return s.nextLocked()
		}
		for s.replayAfter < s.replayUntil {
			seq := s.replayAfter + 1
			rec := b.replay[(b.head+int(seq-oldest))%len(b.replay)]
			s.replayAfter = seq
			if util.GlobMatch(rec.pattern, s.route) {
				return rec.event, true, nil
			}
		}
	}
	if s.count == 0 {
		return Event{}, false, nil
	}
	rec := s.queue[s.head]
	s.queue[s.head] = record{}
	s.head = (s.head + 1) % len(s.queue)
	s.count--
	b.stats.QueueDepth--
	b.stats.QueueBytes -= rec.bytes
	return rec.event, true, nil
}

func (s *Subscription) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Subscription) enqueue(rec record) {
	if s.count == len(s.queue) {
		s.bus.stats.Drops += uint64(s.count) + 1
		s.reset("overflow")
		return
	}
	// The queue needs only delivery data; the routing pattern remains owned by replay.
	rec.pattern = ""
	s.queue[(s.head+s.count)%len(s.queue)] = rec
	s.count++
	s.bus.stats.QueueDepth++
	s.bus.stats.QueueBytes += rec.bytes
	s.notify()
}

func (s *Subscription) reset(reason string) {
	s.control = resetEvent(reason, s.bus.cursor())
	s.terminal = true
	s.bus.stats.Resets++
	s.clearQueue()
	s.notify()
}

// Fail discards pending application events, sends the shared normalized failure, and terminates. A subscription that already closed or carries a priority reset (overflow/replayExpired) keeps that control and returns io.EOF. If the failure exceeds the frame limit it closes immediately and returns an error.
func (s *Subscription) Fail(err error) error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if s.closed || s.terminal {
		return io.EOF
	}
	event, encodeErr := failureEvent(err, s.bus.limits.MaxEventBytes)
	if encodeErr != nil {
		s.closeLocked()
		return encodeErr
	}
	s.terminate(event)
	return nil
}

// Fail terminates every current subscription matching a trusted publisher pattern with a priority failure. Failures are connection controls and are never retained for replay.
func (b *Bus) Fail(ctx context.Context, pattern string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if scopeErr := validateKey(pattern, true); scopeErr != nil {
		return scopeErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return apperrors.ServiceUnavailable("SSE bus")
	}
	event, encodeErr := failureEvent(err, b.limits.MaxEventBytes)
	if encodeErr != nil {
		return encodeErr
	}
	for sub := range b.subs {
		if util.GlobMatch(pattern, sub.route) {
			sub.terminate(event)
		}
	}
	return nil
}

func (s *Subscription) terminate(event Event) {
	if s.closed || s.terminal {
		return
	}
	s.control = event
	s.terminal = true
	s.clearQueue()
	s.notify()
}

func failureEvent(err error, limit int) (Event, error) {
	if util.IsNil(err) {
		return Event{}, apperrors.InvalidInput("failure", "SSE failure is required")
	}
	failure := apperrors.Normalize(err).ToFailure()
	remaining := limit
	for _, field := range []string{string(failure.Code), failure.Message, failure.Reason, failure.TraceID} {
		if len(field) > remaining {
			return Event{}, apperrors.InvalidInput("failure", "SSE failure exceeds the frame limit")
		}
		remaining -= len(field)
	}
	for _, violation := range failure.Violations {
		for _, field := range []string{violation.Field, string(violation.Reason), violation.Message} {
			if len(field)+1 > remaining {
				return Event{}, apperrors.InvalidInput("failure", "SSE failure exceeds the frame limit")
			}
			remaining -= len(field) + 1
		}
	}
	event, encodeErr := controlEvent("failure", failure)
	if encodeErr != nil {
		return Event{}, encodeErr
	}
	if event.size() > limit {
		return Event{}, apperrors.InvalidInput("failure", "SSE failure exceeds the frame limit")
	}
	return event, nil
}

func (s *Subscription) clearQueue() {
	for s.count > 0 {
		rec := &s.queue[s.head]
		s.bus.stats.QueueBytes -= rec.bytes
		s.bus.stats.QueueDepth--
		*rec = record{}
		s.head = (s.head + 1) % len(s.queue)
		s.count--
	}
}

// Close releases admission, queued data, and the context callback. It is idempotent.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	s.closeLocked()
}

func (s *Subscription) closeLocked() {
	if s.closed {
		return
	}
	s.closed, s.terminal = true, true
	s.clearQueue()
	s.queue = nil
	s.control = Event{}
	delete(s.bus.subs, s)
	s.bus.principals[s.principal]--
	if s.bus.principals[s.principal] == 0 {
		delete(s.bus.principals, s.principal)
	}
	if s.stop != nil {
		s.stop()
	}
	s.notify()
}
