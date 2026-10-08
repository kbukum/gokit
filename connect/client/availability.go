package client

import (
	"context"
	"errors"
	"io"
	"sync"

	"connectrpc.com/connect"

	"github.com/kbukum/gokit/component"
)

// AvailabilityState is what a client last observed about its peer.
type AvailabilityState string

// Availability states.
const (
	// AvailabilityUnknown means no call has finished yet.
	AvailabilityUnknown AvailabilityState = "unknown"
	// AvailabilityAvailable means the peer last answered, even with an error.
	AvailabilityAvailable AvailabilityState = "available"
	// AvailabilityUnavailable means the last call could not reach the peer or the peer was overloaded.
	AvailabilityUnavailable AvailabilityState = "unavailable"
)

// Unreachable reports whether err means the peer did not answer: a transport failure, or the Connect codes Unavailable, DeadlineExceeded or Canceled. A connection lost mid-stream can surface under any Connect code, so the transport mark decides first. An error that is not a Connect error is treated as unreachable; any other Connect code is an answer from the peer.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	if IsTransportFailure(err) {
		return true
	}
	var remote *connect.Error
	if !errors.As(err, &remote) {
		return true
	}
	switch remote.Code() {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeCanceled:
		return true
	default:
		return false
	}
}

// Availability records whether a peer answered, from real calls only; it never probes. Install it with connect.WithInterceptors on every client for one peer, and report it through the Health of the component that owns those clients. A degraded health means the peer was unreachable on its last call.
type Availability struct {
	name string

	mu    sync.Mutex
	state AvailabilityState
}

var _ connect.Interceptor = (*Availability)(nil)

// NewAvailability starts as unknown. name is the peer name used in health reports.
func NewAvailability(name string) *Availability {
	return &Availability{name: name, state: AvailabilityUnknown}
}

// State returns the last observed state.
func (a *Availability) State() AvailabilityState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// Observe records the outcome of a finished call made with ctx. A nil error or an answer from the peer marks it available; an [Unreachable] error marks it unavailable. A call that failed after ctx ended, by the caller's cancellation or deadline, says nothing about the peer and is ignored, so callers cannot mark a peer unavailable by choosing a short deadline. Adapters call it directly for outcomes the interceptor cannot see, such as a stream that ended before its first message when the protocol requires one.
func (a *Availability) Observe(ctx context.Context, err error) {
	if err != nil && ctx.Err() != nil {
		return
	}
	state := AvailabilityAvailable
	if Unreachable(err) {
		state = AvailabilityUnavailable
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
}

// Health reports the state as component health named after the peer: degraded when unavailable, otherwise healthy. The message is the state.
func (a *Availability) Health(context.Context) component.Health {
	state := a.State()
	if state == AvailabilityUnavailable {
		return component.Degraded(a.name, string(state))
	}
	h := component.Healthy(a.name)
	h.Message = string(state)
	return h
}

// WrapUnary observes each unary call.
func (a *Availability) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		a.Observe(ctx, err)
		return resp, err
	}
}

// WrapStreamingClient observes received messages, the end of the stream and send failures.
func (a *Availability) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		return &observedConn{StreamingClientConn: next(ctx, spec), ctx: ctx, availability: a}
	}
}

// WrapStreamingHandler leaves server-side streams untouched.
func (a *Availability) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

type observedConn struct {
	connect.StreamingClientConn
	ctx          context.Context //nolint:containedctx // the stream's lifetime is the context it was opened with
	availability *Availability
}

// Receive marks a message or a clean end as an answer and a failure by its kind.
func (c *observedConn) Receive(msg any) error {
	err := c.StreamingClientConn.Receive(msg)
	if errors.Is(err, io.EOF) {
		c.availability.Observe(c.ctx, nil)
	} else {
		c.availability.Observe(c.ctx, err)
	}
	return err
}

// Send observes failures only. io.EOF means the peer closed the stream; Receive reports why.
func (c *observedConn) Send(msg any) error {
	err := c.StreamingClientConn.Send(msg)
	if err != nil && !errors.Is(err, io.EOF) {
		c.availability.Observe(c.ctx, err)
	}
	return err
}
