package sse

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"google.golang.org/protobuf/types/known/apipb"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestLimitsExactFrameAndReplayBytes(t *testing.T) {
	t.Parallel()
	probe := testBus(t, smallLimits())
	sub := subscribe(t, probe, "a", "a", "")
	publish(t, probe, "a", strings.Repeat("x", 200))
	frame := next(t, sub)
	size := frame.size()
	if size != len(frame.Wire()) {
		t.Fatal("frame size accounting differs from wire")
	}
	for _, delta := range []int{-1, 0} {
		limits := smallLimits()
		limits.MaxEventBytes = size + delta
		b := testBus(t, limits)
		err := b.Publish(t.Context(), "a", &apipb.Method{Name: strings.Repeat("x", 200), RequestTypeUrl: "request"})
		if (err == nil) != (delta == 0) {
			t.Fatalf("frame boundary delta %d: %v", delta, err)
		}
	}
	limits := smallLimits()
	limits.ReplayBytes = 2 * (size + 1)
	b := testBus(t, limits)
	cursor := b.Cursor()
	for range 2 {
		publish(t, b, "a", strings.Repeat("x", 200))
	}
	if got := b.Stats(); got.ReplayBytes != limits.ReplayBytes || got.ReplayEvents != 2 {
		t.Fatalf("exact replay bytes: %+v", got)
	}
	publish(t, b, "a", strings.Repeat("x", 201))
	if got := b.Stats(); got.ReplayBytes > limits.ReplayBytes || got.ReplayEvents != 1 {
		t.Fatalf("one extra byte did not evict: %+v", got)
	}
	resumed := subscribe(t, b, "a", "a", cursor)
	if ev := next(t, resumed); ev.Name != "reset" {
		t.Fatalf("byte eviction lost silently: %+v", ev)
	}
}

func TestLimitsReplayLargerThanLiveQueueAndOvertakenReader(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	start := b.Cursor()
	publish(t, b, "a", "one")
	publish(t, b, "a", "two")
	s := subscribe(t, b, "a", "a", start)
	if b.Stats().QueueDepth != 0 {
		t.Fatal("replay copied into live queue")
	}
	if !strings.HasSuffix(next(t, s).ID, ":1") || !strings.HasSuffix(next(t, s).ID, ":2") {
		t.Fatal("replay lost")
	}
	s.Close()
	s = subscribe(t, b, "a", "a", start)
	publish(t, b, "unrelated", "three")
	if ev := next(t, s); ev.Name != "reset" || !strings.Contains(ev.Data, "replayExpired") {
		t.Fatalf("overtaken replay: %+v", ev)
	}
}

func TestLimitsInvalidConfigurationAndPublication(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*Limits){
		func(l *Limits) { l.ReplayEvents = 0 }, func(l *Limits) { l.ReplayEvents = 1<<20 + 1 },
		func(l *Limits) { l.ReplayBytes = 255 }, func(l *Limits) { l.ReplayBytes = 1<<30 + 1 },
		func(l *Limits) { l.QueueEvents = 0 }, func(l *Limits) { l.QueueEvents = 1<<16 + 1 },
		func(l *Limits) { l.MaxEventBytes = 255 }, func(l *Limits) { l.MaxEventBytes = 1<<20 + 1 },
		func(l *Limits) { l.MaxConnections = 0 }, func(l *Limits) { l.MaxConnections = 1<<20 + 1 },
		func(l *Limits) { l.MaxPerPrincipal = 0 }, func(l *Limits) { l.MaxPerPrincipal = 3 },
	} {
		l := smallLimits()
		mutate(&l)
		if _, err := NewBus(l); err == nil {
			t.Fatalf("invalid limits accepted: %+v", l)
		}
	}
	b := testBus(t, smallLimits())
	for _, pattern := range []string{"", "\n", "\x7f", "\xff", strings.Repeat("x", MaxRoutingBytes+1)} {
		if err := b.Publish(t.Context(), pattern, &apipb.Method{}); err == nil {
			t.Fatal("invalid pattern accepted")
		}
	}
	if err := b.Publish(t.Context(), "a", nil); err == nil {
		t.Fatal("nil proto accepted")
	}
	if err := b.Publish(t.Context(), "a", &apipb.Method{Name: strings.Repeat("x", 2048)}); err == nil {
		t.Fatal("oversized proto accepted")
	}
	if err := b.Publish(t.Context(), "a", &apipb.Method{Name: "\xff"}); err == nil {
		t.Fatal("invalid proto accepted")
	}
	if err := b.Publish(t.Context(), strings.Repeat("x", MaxRoutingBytes), &apipb.Method{}); err != nil {
		t.Fatal(err)
	}
	b.sequence = math.MaxUint64
	if err := b.Publish(t.Context(), "a", &apipb.Method{}); err == nil {
		t.Fatal("sequence wrapped")
	}
}

func TestLimitsRoutingAndFanout(t *testing.T) {
	t.Parallel()
	l := smallLimits()
	l.MaxConnections, l.MaxPerPrincipal = 32, 32
	l.QueueEvents = 2
	b := testBus(t, l)
	subs := make([]*Subscription, 0, 32)
	for range 32 {
		subs = append(subs, subscribe(t, b, "same", "path/a[b]", ""))
	}
	for _, pattern := range []string{"path/*", "path/?[b]"} {
		publish(t, b, pattern, "visible")
	}
	if got := b.Stats(); got.ActiveStreams != 32 || got.QueueDepth != 64 {
		t.Fatalf("fanout: %+v", got)
	}
	for _, s := range subs {
		if !strings.HasSuffix(next(t, s).ID, ":1") || !strings.HasSuffix(next(t, s).ID, ":2") {
			t.Fatal("fanout lost")
		}
		s.Close()
	}
	if got := b.Stats(); got.ActiveStreams != 0 || got.QueueDepth != 0 || got.QueueBytes != 0 {
		t.Fatalf("fanout leaked: %+v", got)
	}
}

func TestLimitsConcurrentOwnership(t *testing.T) {
	t.Parallel()
	l := DefaultLimits()
	l.MaxConnections, l.MaxPerPrincipal = 64, 64
	b := testBus(t, l)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			s, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "p", Route: "r"})
			if err != nil {
				t.Error(err)
				return
			}
			defer s.Close()
			if err := b.Publish(t.Context(), "*", &apipb.Method{}); err != nil {
				t.Error(err)
			}
			s.Close()
			s.Close()
		})
	}
	wg.Wait()
	if got := b.Stats(); got.ActiveStreams != 0 || got.QueueDepth != 0 || got.QueueBytes != 0 {
		t.Fatalf("concurrent leak: %+v", got)
	}
}

func TestLimitsCanceledWaitAndFailedControl(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := testBus(t, smallLimits())
		ctx, cancel := context.WithCancel(t.Context())
		s, err := b.Subscribe(ctx, SubscribeRequest{Principal: "a", Route: "a"})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, err := s.Next(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("canceled Next: %v", err)
			}
		}()
		synctest.Wait()
		cancel()
		<-done
		synctest.Wait()
		if b.Stats().ActiveStreams != 0 {
			t.Fatal("cancellation did not release admission")
		}
	})
	b := testBus(t, smallLimits())
	s := subscribe(t, b, "a", "a", "")
	if err := s.Fail(nil); err == nil {
		t.Fatal("nil failure accepted")
	}
	s = subscribe(t, b, "a", "a", "")
	if err := s.Fail(apperrors.InvalidInput("x", strings.Repeat("x", 2048))); err == nil {
		t.Fatal("unbounded failure")
	}
	if b.Stats().ActiveStreams != 0 {
		t.Fatal("oversized failure kept stream open")
	}
	if err := s.Fail(context.Canceled); err == nil {
		t.Fatal("closed failure accepted")
	}
}

func TestTerminalResetSurvivesFail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		fail func(*Bus, *Subscription) error
	}{
		{"subscription fail", func(_ *Bus, s *Subscription) error { return s.Fail(context.Canceled) }},
		{"bus fail", func(b *Bus, _ *Subscription) error { return b.Fail(context.Background(), "*", context.Canceled) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := testBus(t, smallLimits())
			s := subscribe(t, b, "a", "a", "")
			publish(t, b, "a", "one")
			publish(t, b, "a", "two") // fills the one-slot queue and parks a priority overflow reset
			if err := tc.fail(b, s); err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("fail on terminal subscription: %v", err)
			}
			ev := next(t, s)
			if ev.Name != "reset" || !strings.Contains(ev.Data, "overflow") {
				t.Fatalf("priority overflow reset overwritten by failure: %+v", ev)
			}
		})
	}
}

func TestLimitsMalformedCursorCanonicality(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	for _, seq := range []string{"", "+1", "-1", "01", "18446744073709551616", "1:2"} {
		if _, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "a", Route: "a", Cursor: b.epoch + ":" + seq}); err == nil {
			t.Fatalf("accepted cursor %q", seq)
		}
	}
}

func TestComponentOwnership(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	if _, err := NewComponent(nil); err == nil {
		t.Fatal("nil bus accepted")
	}
	c, err := NewComponent(b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c.Name() != "sse" || c.Describe().Type != "sse" || c.Health(t.Context()).Message != "0 streams" {
		t.Fatal("component metadata")
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(t.Context()); err == nil {
		t.Fatal("restarted closed bus")
	}
}
