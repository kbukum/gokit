package client

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
)

// ErrUnaryTimeout reports a unary call whose peer did not answer within the [UnaryTimeoutInterceptor] limit.
var ErrUnaryTimeout = errors.New("connect client: peer did not answer in time")

// UnaryTimeoutInterceptor returns an interceptor that bounds each unary client call by limit. The bound also reaches the peer as the call's timeout header.
//
// A bound applied with context.WithTimeout before the call is indistinguishable from the caller's own deadline, so a hung peer would read as the caller's timeout and never as an outage. Here, when the limit passes while the caller's context is still live, the call returns a Connect deadline_exceeded error wrapping [ErrUnaryTimeout], which [IsUnavailable] counts and [MapCallFailure] maps to ServiceUnavailable. Install it after [Availability] in connect.WithInterceptors, so the availability observes that error against the caller's context. A caller's own cancellation or deadline is left as it is. Streams pass through; bound them with [FirstMessageTimeoutInterceptor].
func UnaryTimeoutInterceptor(limit time.Duration) (connect.Interceptor, error) {
	if limit <= 0 {
		return nil, errors.New("connect client: unary timeout needs a positive limit")
	}
	return unaryTimeout(limit), nil
}

type unaryTimeout time.Duration

func (u unaryTimeout) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !req.Spec().IsClient {
			return next(ctx, req)
		}
		bounded, cancel := context.WithTimeoutCause(ctx, time.Duration(u), ErrUnaryTimeout)
		defer cancel()
		resp, err := next(bounded, req)
		if err != nil && !deadlineReached(ctx) && (errors.Is(context.Cause(bounded), ErrUnaryTimeout) || limitEnforced(ctx, bounded, err)) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, ErrUnaryTimeout)
		}
		return resp, err
	}
}

// limitEnforced reports a deadline_exceeded answer to the limit itself: the limit is the deadline the peer received and
// it has been reached, though the bounded context's timer may not have fired yet.
func limitEnforced(caller, bounded context.Context, err error) bool {
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded || !deadlineReached(bounded) {
		return false
	}
	limit, _ := bounded.Deadline()
	own, ok := caller.Deadline()
	return !ok || limit.Before(own)
}

func (unaryTimeout) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (unaryTimeout) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
