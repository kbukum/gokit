package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kbukum/gokit/util"
)

const (
	feedProcedure = "/test.v1.Feed/Watch"
	firstLimit    = time.Second
)

// armedClock lets tests advance time only after the stream arms its timeout.
type armedClock struct {
	*util.FakeClock
	armed chan struct{}
}

func (c armedClock) NewTimer(d time.Duration) util.Timer {
	timer := c.FakeClock.NewTimer(d)
	c.armed <- struct{}{}
	return timer
}

func newArmedClock() armedClock {
	return armedClock{FakeClock: util.NewFakeClock(time.Unix(0, 0)), armed: make(chan struct{}, 1)}
}

// feed serves one server stream; send controls what the peer does after it accepts the stream.
func feed(t *testing.T, send func(context.Context, *connect.ServerStream[wrapperspb.StringValue]) error) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(feedProcedure, connect.NewServerStreamHandler(feedProcedure,
		func(ctx context.Context, _ *connect.Request[wrapperspb.StringValue], stream *connect.ServerStream[wrapperspb.StringValue]) error {
			return send(ctx, stream)
		}))
	return h2cServer(t, mux.ServeHTTP).URL
}

// hung accepts the stream and never answers until the test ends.
func hung(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})
	url := feed(t, func(ctx context.Context, _ *connect.ServerStream[wrapperspb.StringValue]) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	})
	t.Cleanup(func() { close(release) })
	return url
}

func watchClient(t *testing.T, url string, clock util.TimerClock) (*connect.Client[wrapperspb.StringValue, wrapperspb.StringValue], *Availability) {
	t.Helper()
	limit, err := FirstMessageTimeoutInterceptor(firstLimit, clock)
	if err != nil {
		t.Fatal(err)
	}
	availability := NewAvailability("feed")
	return connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](
		h2cClient(t), url+feedProcedure, connect.WithInterceptors(availability, limit),
	), availability
}

// watch opens the stream and reads the first message in the background, because a hung peer holds the open itself.
func watch(ctx context.Context, c *connect.Client[wrapperspb.StringValue, wrapperspb.StringValue]) <-chan error {
	done := make(chan error, 1)
	go func() {
		stream, err := c.CallServerStream(ctx, connect.NewRequest(wrapperspb.String("")))
		if err != nil {
			done <- err
			return
		}
		defer stream.Close()
		if !stream.Receive() {
			err = stream.Err()
			if err == nil {
				err = io.EOF
			}
		}
		done <- err
	}()
	return done
}

func TestFirstMessageTimeoutRequiresLimitAndClock(t *testing.T) {
	if _, err := FirstMessageTimeoutInterceptor(0, util.SystemClock{}); err == nil {
		t.Fatal("zero limit accepted")
	}
	if _, err := FirstMessageTimeoutInterceptor(time.Second, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
}

func TestFirstMessageTimeoutEndsAHungStreamAsAnOutage(t *testing.T) {
	clock := newArmedClock()
	c, availability := watchClient(t, hung(t), clock)
	done := watch(t.Context(), c)
	<-clock.armed
	clock.Advance(firstLimit)
	err := <-done
	if !errors.Is(err, ErrFirstMessageTimeout) || connect.CodeOf(err) != connect.CodeDeadlineExceeded {
		t.Fatalf("err = %v, want deadline_exceeded with ErrFirstMessageTimeout", err)
	}
	assertState(t, availability, AvailabilityUnavailable)
}

func TestFirstMessageTimeoutLetsATimelyStreamRun(t *testing.T) {
	clock := newArmedClock()
	url := feed(t, func(_ context.Context, stream *connect.ServerStream[wrapperspb.StringValue]) error {
		for _, v := range []string{"a", "b"} {
			if err := stream.Send(wrapperspb.String(v)); err != nil {
				return err
			}
		}
		return nil
	})
	c, availability := watchClient(t, url, clock)
	stream, err := c.CallServerStream(t.Context(), connect.NewRequest(wrapperspb.String("")))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	<-clock.armed
	if !stream.Receive() {
		t.Fatalf("first message: %v", stream.Err())
	}
	// The limit no longer applies once the first message arrived.
	clock.Advance(firstLimit)
	if !stream.Receive() || stream.Msg().GetValue() != "b" {
		t.Fatalf("second message: %v", stream.Err())
	}
	if stream.Receive() || stream.Err() != nil {
		t.Fatalf("end = %v", stream.Err())
	}
	assertState(t, availability, AvailabilityAvailable)
}

func TestFirstMessageTimeoutLeavesCallerCancellationAlone(t *testing.T) {
	clock := newArmedClock()
	c, availability := watchClient(t, hung(t), clock)
	ctx, cancel := context.WithCancel(t.Context())
	done := watch(ctx, c)
	<-clock.armed
	cancel()
	if err := <-done; connect.CodeOf(err) != connect.CodeCanceled || errors.Is(err, ErrFirstMessageTimeout) {
		t.Fatalf("err = %v, want canceled", err)
	}
	clock.Advance(firstLimit)
	assertState(t, availability, AvailabilityUnknown)
}

func TestFirstMessageTimeoutSkipsUnaryAndClientStreams(t *testing.T) {
	limit, err := FirstMessageTimeoutInterceptor(firstLimit, newArmedClock())
	if err != nil {
		t.Fatal(err)
	}
	conn := &fakeConn{}
	got := limit.WrapStreamingClient(func(context.Context, connect.Spec) connect.StreamingClientConn { return conn })(
		t.Context(), connect.Spec{StreamType: connect.StreamTypeClient})
	if got != conn {
		t.Fatal("client stream was wrapped")
	}
	served := errors.New("served")
	unary := connect.UnaryFunc(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, served })
	if _, err := limit.WrapUnary(unary)(t.Context(), nil); !errors.Is(err, served) {
		t.Fatalf("unary = %v", err)
	}
	handler := limit.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error { return served })
	if err := handler(t.Context(), nil); !errors.Is(err, served) {
		t.Fatalf("handler = %v", err)
	}
}

// expiredConn wraps conn for a stream opened with parent whose limit has already passed.
func expiredConn(parent context.Context, conn connect.StreamingClientConn) (*firstMessageConn, util.Timer) {
	ctx, cancel := context.WithCancelCause(parent)
	c := newFirstMessageConn(ctx, cancel)
	c.StreamingClientConn = conn
	return c, util.NewFakeClock(time.Unix(0, 0)).NewTimer(0)
}

func TestFirstMessageTimeoutKeepsCancellationThatBeatTheLimit(t *testing.T) {
	// Both the caller's cancellation and the limit are pending when the watchdog runs, so either select branch may fire.
	for range 64 {
		parent, cancel := context.WithCancel(t.Context())
		cancel()
		c, timer := expiredConn(parent, &fakeConn{receive: []error{connect.NewError(connect.CodeCanceled, context.Canceled)}})
		c.watch(timer)
		if err := c.Receive(nil); errors.Is(err, ErrFirstMessageTimeout) || connect.CodeOf(err) != connect.CodeCanceled {
			t.Fatalf("receive = %v, want the caller's cancellation", err)
		}
		if err := c.Send(nil); errors.Is(err, ErrFirstMessageTimeout) {
			t.Fatalf("send = %v, want the caller's cancellation", err)
		}
	}
}

func TestFirstMessageTimeoutIsStickyAfterTheLimit(t *testing.T) {
	underlying := &fakeConn{receive: []error{nil}, send: []error{nil}}
	c, timer := expiredConn(t.Context(), underlying)
	c.watch(timer)
	for name, call := range map[string]func() error{
		"send":           func() error { return c.Send(nil) },
		"close request":  c.CloseRequest,
		"close response": c.CloseResponse,
		"receive":        func() error { return c.Receive(nil) },
	} {
		if err := call(); !errors.Is(err, ErrFirstMessageTimeout) || connect.CodeOf(err) != connect.CodeDeadlineExceeded {
			t.Errorf("%s = %v, want deadline_exceeded with ErrFirstMessageTimeout", name, err)
		}
	}
	if underlying.closed != 1 {
		t.Fatalf("response cleanup calls = %d, want 1", underlying.closed)
	}
}

func TestFirstMessageTimeoutBoundsStreamConstruction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := util.NewFakeClock(time.Unix(0, 0))
		limit, err := FirstMessageTimeoutInterceptor(firstLimit, clock)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan connect.StreamingClientConn, 1)
		next := func(ctx context.Context, _ connect.Spec) connect.StreamingClientConn {
			<-ctx.Done()
			return &fakeConn{}
		}
		go func() {
			done <- limit.WrapStreamingClient(next)(ctx, connect.Spec{StreamType: connect.StreamTypeServer})
		}()
		synctest.Wait()
		clock.Advance(firstLimit)
		synctest.Wait()
		select {
		case conn := <-done:
			if err := conn.CloseResponse(); !errors.Is(err, ErrFirstMessageTimeout) {
				t.Errorf("constructor timeout = %v", err)
			}
		default:
			t.Error("the first-message limit did not release stream construction")
			cancel()
			synctest.Wait()
			_ = (<-done).CloseResponse()
		}
	})
}
