package client

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
		c := &firstMessageConn{ctx: streamCtx, cancel: cancel, disarmed: make(chan struct{})}
		timer := f.clock.NewTimer(f.limit)
		c.StreamingClientConn = next(streamCtx, spec)
		go c.watch(timer)
		return c
	}
}

// First-message states. Only one transition out of waiting happens, so a message and the limit cannot both win.
const (
	waiting int32 = iota
	answered
	late
)

type firstMessageConn struct {
	connect.StreamingClientConn
	ctx        context.Context //nolint:containedctx // the stream's lifetime is the context it was opened with
	cancel     context.CancelCauseFunc
	state      atomic.Int32
	disarmed   chan struct{}
	disarmOnce sync.Once
}

// watch ends the stream when the limit passes before the first message. It exits when the limit passes, the stream is disarmed or its context ends.
func (c *firstMessageConn) watch(timer util.Timer) {
	defer timer.Stop()
	select {
	case <-timer.C():
		if c.state.CompareAndSwap(waiting, late) {
			c.cancel(ErrNoFirstMessage)
		}
	case <-c.disarmed:
	case <-c.ctx.Done():
	}
}

func (c *firstMessageConn) disarm() {
	c.disarmOnce.Do(func() { close(c.disarmed) })
}

// failure replaces any error after the limit passed, which is the stream's own cancellation, with the outage it stands for.
func (c *firstMessageConn) failure(err error) error {
	if err != nil && c.state.Load() == late {
		return connect.NewError(connect.CodeDeadlineExceeded, ErrNoFirstMessage)
	}
	return err
}

func (c *firstMessageConn) Send(msg any) error {
	return c.failure(c.StreamingClientConn.Send(msg))
}

func (c *firstMessageConn) CloseRequest() error {
	return c.failure(c.StreamingClientConn.CloseRequest())
}

func (c *firstMessageConn) Receive(msg any) error {
	err := c.StreamingClientConn.Receive(msg)
	if c.state.Load() == waiting {
		// A first message, a clean end or a peer error all settle the wait.
		if c.state.CompareAndSwap(waiting, answered) {
			c.disarm()
		}
	}
	if c.state.Load() == late {
		return connect.NewError(connect.CodeDeadlineExceeded, ErrNoFirstMessage)
	}
	return err
}

// CloseResponse stops the limit and releases the stream context.
func (c *firstMessageConn) CloseResponse() error {
	c.disarm()
	c.cancel(nil)
	return c.StreamingClientConn.CloseResponse()
}
