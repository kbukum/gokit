package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"

	apperrors "github.com/kbukum/gokit/errors"
)

const echoProcedure = "/test.v1.Echo/Say"

// echo serves one unary procedure; answer decides what the peer does with each call.
func echo(t *testing.T, answer func(context.Context) error) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(echoProcedure, connect.NewUnaryHandler(echoProcedure,
		func(ctx context.Context, req *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
			if err := answer(ctx); err != nil {
				return nil, err
			}
			return connect.NewResponse(req.Msg), nil
		}))
	return h2cServer(t, mux.ServeHTTP).URL
}

// silent accepts calls and answers none until the call or the test ends.
func silent(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	return echo(t, func(ctx context.Context) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ctx.Err()
	})
}

func sayClient(t *testing.T, url string, limit time.Duration) (*connect.Client[wrapperspb.StringValue, wrapperspb.StringValue], *Availability) {
	t.Helper()
	timeout, err := UnaryTimeoutInterceptor(limit)
	if err != nil {
		t.Fatal(err)
	}
	availability := NewAvailability("echo")
	return connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		h2cClient(t), url+echoProcedure, connect.WithInterceptors(availability, timeout),
	), availability
}

func TestUnaryTimeoutInterceptorValidatesItsLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []time.Duration{0, -time.Second} {
		if _, err := UnaryTimeoutInterceptor(limit); err == nil {
			t.Fatalf("limit %v accepted", limit)
		}
	}
}

func TestUnaryTimeoutIsAPeerOutage(t *testing.T) {
	t.Parallel()
	c, availability := sayClient(t, silent(t), 50*time.Millisecond)
	ctx := t.Context()
	_, err := c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("hi")))
	if !errors.Is(err, ErrUnaryTimeout) || connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Fatalf("err = %v, want deadline_exceeded wrapping ErrUnaryTimeout", err)
	}
	if got := availability.State(); got != AvailabilityUnavailable {
		t.Fatalf("availability = %s, want unavailable", got)
	}
	mapped, ok := MapCallFailure(ctx, "echo", err)
	if !ok || mapped.Code != apperrors.ErrCodeServiceUnavailable || mapped.Reason != ReasonUnavailable {
		t.Fatalf("mapped = %v, %v; want a peer outage", mapped, ok)
	}
}

func TestUnaryTimeoutLeavesTheCallersDeadlineAlone(t *testing.T) {
	t.Parallel()
	c, availability := sayClient(t, silent(t), time.Minute)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := c.CallUnary(ctx, connect.NewRequest(wrapperspb.String("hi")))
	if err == nil || errors.Is(err, ErrUnaryTimeout) {
		t.Fatalf("err = %v, want the caller's own deadline", err)
	}
	if got := availability.State(); got != AvailabilityUnknown {
		t.Fatalf("availability = %s, want unknown: the caller's deadline says nothing about the peer", got)
	}
	mapped, ok := MapCallFailure(ctx, "echo", err)
	if !ok || mapped.Code != apperrors.ErrCodeTimeout {
		t.Fatalf("mapped = %v, %v; want the caller's timeout", mapped, ok)
	}
}

func TestUnaryTimeoutPassesAnswersThrough(t *testing.T) {
	t.Parallel()
	refused := connect.NewError(connect.CodePermissionDenied, errors.New("no"))
	url := echo(t, func(ctx context.Context) error {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Minute {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("the limit did not reach the peer"))
		}
		return nil
	})
	c, availability := sayClient(t, url, time.Minute)
	resp, err := c.CallUnary(t.Context(), connect.NewRequest(wrapperspb.String("hi")))
	if err != nil || resp.Msg.GetValue() != "hi" {
		t.Fatalf("call = %v, %v", resp, err)
	}
	if got := availability.State(); got != AvailabilityAvailable {
		t.Fatalf("availability = %s, want available", got)
	}

	denied, _ := sayClient(t, echo(t, func(context.Context) error { return refused }), time.Minute)
	if _, err := denied.CallUnary(t.Context(), connect.NewRequest(wrapperspb.String("hi"))); connect.CodeOf(err) != connect.CodePermissionDenied || errors.Is(err, ErrUnaryTimeout) {
		t.Fatalf("err = %v, want the peer's own answer", err)
	}
}

func TestUnaryTimeoutIgnoresStreams(t *testing.T) {
	t.Parallel()
	timeout, err := UnaryTimeoutInterceptor(time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	streamed := false
	timeout.WrapStreamingClient(func(ctx context.Context, _ connect.Spec) connect.StreamingClientConn {
		_, bounded := ctx.Deadline()
		streamed = !bounded
		return nil
	})(t.Context(), connect.Spec{StreamType: connect.StreamTypeServer})
	if !streamed {
		t.Fatal("a stream was bounded by the unary limit")
	}
	handler := timeout.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error { return nil })
	if handler == nil {
		t.Fatal("handler wrapper is nil")
	}
}

// The peer receives the limit truncated to milliseconds, so its deadline_exceeded can arrive before the limit's timer.
func TestLimitEnforcedRecognizesThePeerHonouringTheLimit(t *testing.T) {
	t.Parallel()
	enforced := peerError(t, connect.NewError(connect.CodeDeadlineExceeded, errors.New("propagated limit")))
	near := func(parent context.Context) context.Context {
		ctx, cancel := context.WithTimeout(parent, deadlinePrecision/2)
		t.Cleanup(cancel)
		return ctx
	}
	distant, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	earlier, cancelEarlier := context.WithTimeout(context.Background(), deadlinePrecision/4)
	defer cancelEarlier()
	for name, tc := range map[string]struct {
		caller, bounded context.Context
		err             error
		want            bool
	}{
		"limit reached":                       {context.Background(), near(context.Background()), enforced, true},
		"limit under a later caller deadline": {distant, near(distant), enforced, true},
		"limit not reached":                   {context.Background(), distant, enforced, false},
		"caller deadline first":               {earlier, near(earlier), enforced, false},
		"another answer":                      {context.Background(), near(context.Background()), peerError(t, connect.NewError(connect.CodeUnavailable, errors.New("down"))), false},
	} {
		if got := limitEnforced(tc.caller, tc.bounded, tc.err); got != tc.want {
			t.Errorf("%s: limitEnforced = %v, want %v", name, got, tc.want)
		}
	}
}
