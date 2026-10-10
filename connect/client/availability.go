package client

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

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

// IsUnavailable reports a marked transport failure, a context, unary or first-message timeout, or a received Connect Unavailable, DeadlineExceeded or Canceled. Other locally constructed errors carry no evidence about the peer, regardless of their code. Nil and the clean io.EOF sentinel are successful outcomes; a marked failure wrapping EOF remains an outage.
func IsUnavailable(err error) bool {
	return callAvailability(err) == AvailabilityUnavailable
}

func callAvailability(err error) AvailabilityState {
	if err == nil || err == io.EOF { //nolint:errorlint // only the clean sentinel is completion; wrapped EOF may be a transport failure
		return AvailabilityAvailable
	}
	if IsTransportFailure(err) || errors.Is(err, ErrFirstMessageTimeout) || errors.Is(err, ErrUnaryTimeout) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return AvailabilityUnavailable
	}
	var remote *connect.Error
	if !errors.As(err, &remote) || !connect.IsWireError(err) {
		return AvailabilityUnknown
	}
	switch remote.Code() {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeCanceled:
		return AvailabilityUnavailable
	default:
		return AvailabilityAvailable
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

// Observe records a finished call made with ctx. Success, clean EOF and non-outage wire errors mark the peer available. An [IsUnavailable] failure marks it unavailable unless ctx ended or reached its deadline. Other local errors leave the previous state unchanged: a local rejection is not a peer answer. Adapters may record outcomes the interceptor cannot see.
func (a *Availability) Observe(ctx context.Context, err error) {
	state := callAvailability(err)
	if state == AvailabilityUnknown || (state == AvailabilityUnavailable && deadlineReached(ctx)) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
}

// deadlinePrecision is how much earlier a peer's copy of a propagated deadline can end than the caller's own: Connect
// sends the remaining time truncated to whole milliseconds.
const deadlinePrecision = time.Millisecond

// deadlineReached reports whether ctx ended or is within deadlinePrecision of its deadline. A peer enforcing the
// propagated deadline can answer deadline_exceeded before the caller's timer fires, and a loaded runtime can fire that
// timer late; either way the failure reflects the caller's budget, not the peer.
func deadlineReached(ctx context.Context) bool {
	return ctx.Err() != nil || deadlineDue(ctx)
}

// deadlineDue reports whether ctx's deadline is within deadlinePrecision, whether or not its timer has fired.
func deadlineDue(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	return ok && !time.Now().Add(deadlinePrecision).Before(deadline)
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
	c.availability.Observe(c.ctx, err)
	return err
}

// Send observes failures only. io.EOF means the peer closed the stream; Receive reports why.
func (c *observedConn) Send(msg any) error {
	err := c.StreamingClientConn.Send(msg)
	if err != nil && err != io.EOF { //nolint:errorlint // a wrapped EOF can mark a transport failure
		c.availability.Observe(c.ctx, err)
	}
	return err
}
