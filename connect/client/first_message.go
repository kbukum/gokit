package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"connectrpc.com/connect"

	"github.com/kbukum/gokit/util"
)

// ErrNoFirstMessage reports a stream whose peer sent no first message within the [FirstMessageTimeout] limit.
var ErrNoFirstMessage = errors.New("connect client: stream sent no first message in time")

// FirstMessageTimeout returns an interceptor that bounds how long a server or bidi stream waits for its first response message. A client built with [Config.NoTimeout] has no other bound, and a peer that accepts the connection but never answers would hold the stream for as long as the caller waits. The clock starts when the stream is created, so it covers opening the stream too: connect-go waits for the response headers before the open returns.
//
// When the limit passes first, the stream ends and every later call on it returns a Connect deadline_exceeded error wrapping [ErrNoFirstMessage], which [IsUnavailable] counts as an outage; a message that races the limit is dropped. Install it after [Availability] in connect.WithInterceptors, so the availability observes that error against the caller's context. A caller's own cancellation or deadline is left as it is. Unary calls and client streams pass through.
func FirstMessageTimeout(limit time.Duration, clock util.TimerClock) (connect.Interceptor, error) {
	if limit <= 0 || util.IsNil(clock) {
		return nil, errors.New("connect client: first message timeout needs a positive limit and a clock")
	}
	return &firstMessage{limit: limit, clock: clock}, nil
}

type firstMessage struct {
	limit time.Duration
	clock util.TimerClock
}

func (f *firstMessage) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }

func (f *firstMessage) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func (f *firstMessage) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		if spec.StreamType&connect.StreamTypeServer == 0 {
			return next(ctx, spec)
		}
		streamCtx, cancel := context.WithCancelCause(ctx)
		c := newFirstMessageConn(streamCtx, cancel)
		timer := f.clock.NewTimer(f.limit)
		c.StreamingClientConn = next(streamCtx, spec)
		go c.watch(timer)
		return c
	}
}

// First-message states. Only one transition out of waiting happens, so the first message, the caller and the limit cannot both win.
const (
	waiting int32 = iota
	settled
	late
)

type firstMessageConn struct {
	connect.StreamingClientConn
	ctx        context.Context //nolint:containedctx // the stream's lifetime is the context it was opened with
	cancel     context.CancelCauseFunc
	mu         sync.Mutex
	state      int32 // guarded by mu
	disarmed   chan struct{}
	disarmOnce sync.Once
}

func newFirstMessageConn(ctx context.Context, cancel context.CancelCauseFunc) *firstMessageConn {
	return &firstMessageConn{ctx: ctx, cancel: cancel, disarmed: make(chan struct{})}
}

// watch ends the stream when the limit passes before the first message. It exits when the limit passes, the stream is disarmed or its context ends.
func (c *firstMessageConn) watch(timer util.Timer) {
	defer timer.Stop()
	select {
	case <-timer.C():
		c.expire()
	case <-c.disarmed:
	case <-c.ctx.Done():
	}
}

// expire ends a stream still waiting for its first message. The limit wins only when its own cancellation ended the stream: a caller's cancellation or deadline that got there first keeps its error even when the timer fired too.
func (c *firstMessageConn) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != waiting {
		return
	}
	c.cancel(ErrNoFirstMessage)
	if errors.Is(context.Cause(c.ctx), ErrNoFirstMessage) {
		c.state = late
	} else {
		c.state = settled
	}
}

// settle ends the wait for the first message unless the limit already passed, and reports whether it had.
func (c *firstMessageConn) settle() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == waiting {
		c.state = settled
		c.disarm()
	}
	return c.state == late
}

func (c *firstMessageConn) expired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == late
}

func (c *firstMessageConn) disarm() {
	c.disarmOnce.Do(func() { close(c.disarmed) })
}

// failure replaces the outcome of any call after the limit passed, success included, with the outage it stands for.
func (c *firstMessageConn) failure(err error) error {
	if c.expired() {
		return errNoFirstMessage()
	}
	return err
}

func errNoFirstMessage() error {
	return connect.NewError(connect.CodeDeadlineExceeded, ErrNoFirstMessage)
}

func (c *firstMessageConn) Send(msg any) error {
	return c.failure(c.StreamingClientConn.Send(msg))
}

func (c *firstMessageConn) CloseRequest() error {
	return c.failure(c.StreamingClientConn.CloseRequest())
}

// Receive settles the wait: a first message, a clean end or a peer error all count as an answer.
func (c *firstMessageConn) Receive(msg any) error {
	err := c.StreamingClientConn.Receive(msg)
	if c.settle() {
		return errNoFirstMessage()
	}
	return err
}

// CloseResponse stops the limit and releases the stream context.
func (c *firstMessageConn) CloseResponse() error {
	c.disarm()
	c.cancel(nil)
	return c.StreamingClientConn.CloseResponse()
}
