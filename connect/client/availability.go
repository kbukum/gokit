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
	// AvailabilityAvailable means the peer last served a call, even with an error response.
	AvailabilityAvailable AvailabilityState = "available"
	// AvailabilityUnavailable means the last call counted as [IsUnavailable].
	AvailabilityUnavailable AvailabilityState = "unavailable"
)

// IsUnavailable reports whether err means the peer cannot serve calls right now. That is either a call that never got an answer (a transport failure, or an error that is not a Connect error) or a Connect code saying the peer cannot serve it now: Unavailable, DeadlineExceeded or Canceled, whether the client or the peer produced it. A connection lost mid-stream can surface under any Connect code, so the transport mark decides first. Any other Connect code is a served call: the peer is available and the error is its answer.
func IsUnavailable(err error) bool {
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

// Availability records whether a peer can serve calls, from real calls only; it never probes. Install it with connect.WithInterceptors on every client for one peer, and report it through the Health of the component that owns those clients. A degraded health means the last call counted as [IsUnavailable].
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

// Observe records the outcome of a finished call made with ctx. A nil error or any other error the peer served marks it available; an [IsUnavailable] error marks it unavailable, unless ctx had already ended: a caller's own cancellation or deadline says nothing about the peer, so callers cannot mark a peer unavailable by choosing a short deadline. Adapters call it directly for outcomes the interceptor cannot see, such as a stream that ended before its first message when the protocol requires one.
func (a *Availability) Observe(ctx context.Context, err error) {
	state := AvailabilityAvailable
	if IsUnavailable(err) {
		if ctx.Err() != nil {
			return
		}
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
