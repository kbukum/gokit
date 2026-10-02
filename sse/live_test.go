package sse

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/apipb"

	apperrors "github.com/kbukum/gokit/errors"
)

func smallLimits() Limits {
	return Limits{ReplayEvents: 2, ReplayBytes: 4096, QueueEvents: 1, MaxEventBytes: 1024, MaxConnections: 2, MaxPerPrincipal: 1}
}

func testBus(t *testing.T, limits Limits) *Bus {
	t.Helper()
	b, err := NewBus(limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func subscribe(t *testing.T, b *Bus, principal, route, cursor string) *Subscription {
	t.Helper()
	s, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: principal, Route: route, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func publish(t *testing.T, b *Bus, route, value string) {
	t.Helper()
	if err := b.Publish(t.Context(), route, &apipb.Method{Name: value, RequestTypeUrl: "request"}); err != nil {
		t.Fatal(err)
	}
}

func next(t *testing.T, s *Subscription) Event {
	t.Helper()
	e, err := s.Next(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestLiveScopedReplayAndGlobalGaps(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	start := b.Cursor()
	publish(t, b, "bob", "secret")
	publish(t, b, "alice", "visible")
	s := subscribe(t, b, "alice", "alice", start)
	e := next(t, s)
	if e.Name != "google.protobuf.Method" || !strings.HasSuffix(e.ID, ":2") || strings.Contains(e.Data, "secret") || !strings.Contains(e.Data, `"requestTypeUrl"`) {
		t.Fatalf("wrong scoped proto event: %+v", e)
	}
	s.Close()
	resumed := subscribe(t, b, "alice", "alice", e.ID)
	publish(t, b, "alice", "new")
	if got := next(t, resumed); !strings.HasSuffix(got.ID, ":3") {
		t.Fatalf("duplicate on resume: %+v", got)
	}
}

func TestLiveOverflowHasPriorityAndCloses(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	s := subscribe(t, b, "alice", "alice", "")
	publish(t, b, "alice", "one")
	publish(t, b, "alice", "two")
	if e := next(t, s); e.Name != "reset" || e.ID != "" || !strings.Contains(e.Data, `"overflow"`) {
		t.Fatalf("missing priority reset: %+v", e)
	}
	if _, err := s.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("overflow must close: %v", err)
	}
	s.Close()
	if stats := b.Stats(); stats.ActiveStreams != 0 || stats.QueueBytes != 0 || stats.QueueDepth != 0 || stats.Drops != 2 {
		t.Fatalf("resource accounting: %+v", stats)
	}
}

func TestLiveCursorValidationAndReset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, cursor, reason string }{
		{"malformed", "oops", ""},
		{"future", strings.Repeat("a", 32) + ":99", "epochChanged"},
		{"foreign", strings.Repeat("0", 32) + ":0", "epochChanged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := testBus(t, smallLimits())
			s, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "a", Route: "a", Cursor: tc.cursor})
			if tc.reason == "" {
				if err == nil || b.Stats().ActiveStreams != 0 {
					t.Fatalf("bad cursor allocated: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if e := next(t, s); e.Name != "reset" || !strings.Contains(e.Data, tc.reason) {
				t.Fatalf("reset: %+v", e)
			}
		})
	}
	b := testBus(t, smallLimits())
	start := b.Cursor()
	future := strings.TrimSuffix(start, ":0") + ":1"
	if _, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "a", Route: "a", Cursor: future}); err == nil {
		t.Fatal("same-epoch future cursor accepted")
	}
	for range 3 {
		publish(t, b, "a", "event")
	}
	s := subscribe(t, b, "a", "a", start)
	if e := next(t, s); e.Name != "reset" || !strings.Contains(e.Data, "replayExpired") {
		t.Fatalf("evicted cursor: %+v", e)
	}
}

func TestLiveAdmissionAndFailure(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	a := subscribe(t, b, "a", "route", "")
	if _, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "a", Route: "other"}); err == nil {
		t.Fatal("principal connection limit not enforced")
	}
	subscribe(t, b, "b", "route", "")
	if _, err := b.Subscribe(t.Context(), SubscribeRequest{Principal: "c", Route: "route"}); err == nil {
		t.Fatal("instance connection limit not enforced")
	}
	if stats := b.Stats(); stats.AllocatedQueues != 2 || stats.RejectedConnections != 2 {
		t.Fatalf("rejection allocated queue: %+v", stats)
	}
	publish(t, b, "route", "queued")
	if err := a.Fail(apperrors.InvalidInput("field", "bad")); err != nil {
		t.Fatal(err)
	}
	if e := next(t, a); e.Name != "failure" || !strings.Contains(e.Data, `"retryable":false`) {
		t.Fatalf("priority failure: %+v", e)
	}
}

func TestLiveCancellationAndClose(t *testing.T) {
	t.Parallel()
	b := testBus(t, smallLimits())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := b.Subscribe(ctx, SubscribeRequest{Principal: "a", Route: "a"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission: %v", err)
	}
	s := subscribe(t, b, "a", "a", "")
	b.Close()
	b.Close()
	if _, err := s.Next(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if err := b.Publish(t.Context(), "a", &apipb.Method{}); err == nil {
		t.Fatal("closed publish succeeded")
	}
}
