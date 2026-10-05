package httpclient

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/kbukum/gokit/util"
)

type streamLifetime struct {
	ctx        context.Context
	cancel     context.CancelCauseFunc
	clock      util.TimerClock
	cfg        StreamConfig
	manual     bool
	mu         sync.Mutex
	total      time.Time
	deadline   time.Time
	phase      string
	wake       chan struct{}
	watched    chan struct{}
	finished   chan struct{}
	body       io.ReadCloser
	closeOnce  sync.Once
	closeErr   error
	readMu     sync.Mutex
	finishOnce sync.Once
	release    func(error) error
	outcome    error
}

func newStreamLifetime(ctx context.Context, cfg StreamConfig, clock util.TimerClock, manual bool) *streamLifetime {
	ctx, cancel := context.WithCancelCause(ctx)
	s := &streamLifetime{
		ctx: ctx, cancel: cancel, cfg: cfg, clock: clock, manual: manual,
		total: clock.Now().Add(cfg.TotalTimeout), wake: make(chan struct{}, 1), watched: make(chan struct{}),
		finished: make(chan struct{}),
	}
	go s.watch()
	return s
}

func (s *streamLifetime) setPhase(phase string, duration time.Duration) {
	s.mu.Lock()
	if !s.expireLocked() {
		s.phase, s.deadline = phase, s.clock.Now().Add(duration)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *streamLifetime) progress(model bool) {
	s.mu.Lock()
	if s.expireLocked() {
		s.mu.Unlock()
		return
	}
	if s.phase == "first_progress" && !model {
		s.mu.Unlock()
		return
	}
	s.phase, s.deadline = "idle", s.clock.Now().Add(s.cfg.IdleTimeout)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// expireLocked prevents a late progress call from renewing an already expired budget.
func (s *streamLifetime) expireLocked() bool {
	now := s.clock.Now()
	phase := ""
	if !s.total.After(now) {
		phase = "total"
	} else if !s.deadline.IsZero() && !s.deadline.After(now) {
		phase = s.phase
	}
	if phase != "" {
		s.cancel(streamTimeout(phase))
	}
	return s.ctx.Err() != nil
}

func (s *streamLifetime) attach(body io.ReadCloser) {
	s.mu.Lock()
	s.body = body
	canceled := s.ctx.Err() != nil
	s.mu.Unlock()
	if canceled {
		s.closeBody()
	}
}

func (s *streamLifetime) closeBody() {
	s.mu.Lock()
	body := s.body
	s.mu.Unlock()
	if body != nil {
		s.closeOnce.Do(func() { s.closeErr = body.Close() })
	}
}

func (s *streamLifetime) watch() {
	defer close(s.watched)
	for {
		s.mu.Lock()
		deadline := s.total
		if !s.deadline.IsZero() && s.deadline.Before(deadline) {
			deadline = s.deadline
		}
		s.mu.Unlock()
		timer := s.clock.NewTimer(deadline.Sub(s.clock.Now()))
		select {
		case <-s.ctx.Done():
			timer.Stop()
			s.closeBody()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C():
			// Recheck under the same lock used by progress so a stale timer cannot expire a renewed lease.
			s.mu.Lock()
			expired := s.expireLocked()
			s.mu.Unlock()
			if expired {
				s.closeBody()
				return
			}
		}
	}
}

func (s *streamLifetime) finish(err error) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.expireLocked()
		s.mu.Unlock()
		if cause := context.Cause(s.ctx); cause != nil {
			err = cause
		}
		s.cancel(err)
		s.closeBody()
		s.readMu.Lock()
		defer s.readMu.Unlock()
		<-s.watched
		s.outcome = errors.Join(err, s.closeErr)
		if s.release != nil {
			s.outcome = s.release(s.outcome)
		}
		close(s.finished)
	})
}

func (s *streamLifetime) terminal() (error, bool) {
	select {
	case <-s.finished:
		if s.outcome == nil {
			return io.EOF, true
		}
		return s.outcome, true
	default:
		return nil, false
	}
}

type streamBody struct {
	life   *streamLifetime
	rawEOF bool
}

func (b *streamBody) Read(p []byte) (int, error) {
	s := b.life
	s.readMu.Lock()
	if err, done := s.terminal(); done {
		s.readMu.Unlock()
		return 0, err
	}
	n, err := s.body.Read(p)
	s.readMu.Unlock()
	if cause := context.Cause(s.ctx); cause != nil {
		err = cause
	}
	if !s.manual && b.rawEOF {
		if n > 0 {
			s.progress(true)
		}
		if err != nil {
			outcome := err
			if errors.Is(err, io.EOF) {
				outcome = nil
			}
			s.finish(outcome)
			if s.outcome != nil {
				err = s.outcome
			}
		}
	}
	return n, err
}

func (b *streamBody) Close() error {
	b.life.finish(context.Canceled)
	<-b.life.watched
	return b.life.closeErr
}
