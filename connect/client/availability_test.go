package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/kbukum/gokit/component"
)

func TestIsUnavailableClassifiesPeerFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"transport failure under any code", connect.NewError(connect.CodeInternal, &transportError{cause: errors.New("reset")}), true},
		{"unavailable", connect.NewError(connect.CodeUnavailable, errors.New("down")), true},
		{"deadline exceeded", connect.NewError(connect.CodeDeadlineExceeded, errors.New("slow")), true},
		{"canceled by the peer", connect.NewError(connect.CodeCanceled, errors.New("gone")), true},
		{"not a connect error", errors.New("plain"), true},
		{"answer: permission denied", connect.NewError(connect.CodePermissionDenied, errors.New("no")), false},
		{"answer: resource exhausted", connect.NewError(connect.CodeResourceExhausted, errors.New("slow down")), false},
		{"answer: internal", connect.NewError(connect.CodeInternal, errors.New("bug")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnavailable(tc.err); got != tc.want {
				t.Fatalf("IsUnavailable = %v, want %v", got, tc.want)
			}
		})
	}
	if IsUnavailable(nil) {
		t.Fatal("IsUnavailable(nil) = true, want false")
	}
}

func TestAvailabilityStartsUnknownAndFollowsTraffic(t *testing.T) {
	a := NewAvailability("access")
	ctx := context.Background()
	assertState(t, a, AvailabilityUnknown)

	a.Observe(ctx, connect.NewError(connect.CodeUnavailable, errors.New("down")))
	assertState(t, a, AvailabilityUnavailable)

	a.Observe(ctx, connect.NewError(connect.CodePermissionDenied, errors.New("answered")))
	assertState(t, a, AvailabilityAvailable)

	a.Observe(ctx, connect.NewError(connect.CodeUnavailable, errors.New("down")))
	a.Observe(ctx, nil)
	assertState(t, a, AvailabilityAvailable)
}

func TestAvailabilityIgnoresCallerCancellation(t *testing.T) {
	a := NewAvailability("access")
	a.Observe(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Observe(ctx, connect.NewError(connect.CodeCanceled, context.Canceled))
	assertState(t, a, AvailabilityAvailable)
}

func TestAvailabilityIgnoresCallerDeadline(t *testing.T) {
	a := NewAvailability("access")
	a.Observe(context.Background(), nil)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	a.Observe(ctx, connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded))
	assertState(t, a, AvailabilityAvailable)
}

func TestAvailabilityCountsAnAnswerAfterTheCallerEnded(t *testing.T) {
	a := NewAvailability("access")
	a.Observe(context.Background(), connect.NewError(connect.CodeUnavailable, errors.New("down")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Observe(ctx, connect.NewError(connect.CodePermissionDenied, errors.New("answered")))
	assertState(t, a, AvailabilityAvailable)
}

func TestAvailabilityCountsPeerDeadlineWhileCallerWaits(t *testing.T) {
	a := NewAvailability("access")
	a.Observe(context.Background(), connect.NewError(connect.CodeDeadlineExceeded, errors.New("client timeout")))
	assertState(t, a, AvailabilityUnavailable)
}

func TestAvailabilityHealthReportsState(t *testing.T) {
	a := NewAvailability("events")
	ctx := context.Background()
	h := a.Health(ctx)
	if h.Name != "events" || h.Status != component.StatusHealthy || h.Message != string(AvailabilityUnknown) {
		t.Fatalf("unknown health = %+v", h)
	}
	a.Observe(ctx, connect.NewError(connect.CodeUnavailable, errors.New("down")))
	h = a.Health(ctx)
	if h.Status != component.StatusDegraded || h.Message != string(AvailabilityUnavailable) {
		t.Fatalf("unavailable health = %+v", h)
	}
	a.Observe(ctx, nil)
	if h = a.Health(ctx); h.Status != component.StatusHealthy || h.Message != string(AvailabilityAvailable) {
		t.Fatalf("available health = %+v", h)
	}
}

func TestAvailabilityInterceptorObservesUnaryCalls(t *testing.T) {
	a := NewAvailability("control")
	fail := connect.NewError(connect.CodeUnavailable, errors.New("down"))
	call := a.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, fail })
	if _, err := call(context.Background(), nil); !errors.Is(err, fail) {
		t.Fatalf("err = %v, want the peer error unchanged", err)
	}
	assertState(t, a, AvailabilityUnavailable)

	ok := a.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&struct{}{}), nil
	})
	if _, err := ok(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	assertState(t, a, AvailabilityAvailable)
}

func TestAvailabilityInterceptorObservesStreams(t *testing.T) {
	ctx := context.Background()
	t.Run("first message marks available", func(t *testing.T) {
		a := NewAvailability("events")
		conn := a.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
			return &fakeConn{receive: []error{nil}}
		})(ctx, connect.Spec{})
		if err := conn.Receive(nil); err != nil {
			t.Fatal(err)
		}
		assertState(t, a, AvailabilityAvailable)
	})
	t.Run("clean end is an answer", func(t *testing.T) {
		a := NewAvailability("events")
		a.Observe(ctx, connect.NewError(connect.CodeUnavailable, errors.New("down")))
		conn := a.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
			return &fakeConn{receive: []error{io.EOF}}
		})(ctx, connect.Spec{})
		if err := conn.Receive(nil); !errors.Is(err, io.EOF) {
			t.Fatalf("err = %v, want io.EOF", err)
		}
		assertState(t, a, AvailabilityAvailable)
	})
	t.Run("failure after messages marks unavailable", func(t *testing.T) {
		a := NewAvailability("events")
		lost := connect.NewError(connect.CodeUnknown, &transportError{cause: errors.New("reset")})
		conn := a.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
			return &fakeConn{receive: []error{nil, lost}}
		})(ctx, connect.Spec{})
		_ = conn.Receive(nil)
		if err := conn.Receive(nil); !errors.Is(err, lost) {
			t.Fatalf("err = %v", err)
		}
		assertState(t, a, AvailabilityUnavailable)
	})
	t.Run("send failure marks unavailable, server close does not", func(t *testing.T) {
		a := NewAvailability("events")
		conn := a.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
			return &fakeConn{send: []error{io.EOF, connect.NewError(connect.CodeUnavailable, errors.New("down"))}}
		})(ctx, connect.Spec{})
		_ = conn.Send(nil)
		assertState(t, a, AvailabilityUnknown)
		_ = conn.Send(nil)
		assertState(t, a, AvailabilityUnavailable)
	})
	t.Run("caller cancellation is ignored", func(t *testing.T) {
		a := NewAvailability("events")
		cctx, cancel := context.WithCancel(ctx)
		conn := a.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn {
			return &fakeConn{receive: []error{connect.NewError(connect.CodeCanceled, context.Canceled)}}
		})(cctx, connect.Spec{})
		cancel()
		_ = conn.Receive(nil)
		assertState(t, a, AvailabilityUnknown)
	})
	t.Run("handler side is untouched", func(t *testing.T) {
		a := NewAvailability("events")
		var called bool
		h := a.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error { called = true; return nil })
		if err := h(ctx, nil); err != nil || !called {
			t.Fatalf("handler err = %v called = %v", err, called)
		}
	})
}

func assertState(t *testing.T, a *Availability, want AvailabilityState) {
	t.Helper()
	if got := a.State(); got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
}

type fakeConn struct {
	connect.StreamingClientConn
	receive []error
	send    []error
}

func (c *fakeConn) Receive(any) error   { return pop(&c.receive) }
func (c *fakeConn) Send(any) error      { return pop(&c.send) }
func (c *fakeConn) CloseRequest() error { return nil }
func (c *fakeConn) RequestHeader() http.Header {
	return http.Header{}
}

func pop(errs *[]error) error {
	if len(*errs) == 0 {
		return io.EOF
	}
	err := (*errs)[0]
	*errs = (*errs)[1:]
	return err
}
