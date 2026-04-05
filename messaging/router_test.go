package messaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouter_ExactMatch(t *testing.T) {
	t.Parallel()

	var called string
	r := NewRouter().
		Handle("content.discovered", func(_ context.Context, _ Message) error {
			called = "discovered"
			return nil
		}).
		Handle("content.analyzed", func(_ context.Context, _ Message) error {
			called = "analyzed"
			return nil
		})

	h := r.Handler()

	if err := h(context.Background(), Message{Topic: "content.discovered"}); err != nil {
		t.Fatal(err)
	}
	if called != "discovered" {
		t.Errorf("called = %q, want discovered", called)
	}

	if err := h(context.Background(), Message{Topic: "content.analyzed"}); err != nil {
		t.Fatal(err)
	}
	if called != "analyzed" {
		t.Errorf("called = %q, want analyzed", called)
	}
}

func TestRouter_Wildcard(t *testing.T) {
	t.Parallel()

	var got string
	r := NewRouter().
		Handle("content.*", func(_ context.Context, msg Message) error {
			got = msg.Topic
			return nil
		})

	h := r.Handler()

	if err := h(context.Background(), Message{Topic: "content.discovered"}); err != nil {
		t.Fatal(err)
	}
	if got != "content.discovered" {
		t.Errorf("got = %q, want content.discovered", got)
	}

	if err := h(context.Background(), Message{Topic: "content.analyzed"}); err != nil {
		t.Fatal(err)
	}
	if got != "content.analyzed" {
		t.Errorf("got = %q, want content.analyzed", got)
	}
}

func TestRouter_ExactMatchTakesPrecedenceOverWildcard(t *testing.T) {
	t.Parallel()

	var handler string
	r := NewRouter().
		Handle("content.*", func(_ context.Context, _ Message) error {
			handler = "wildcard"
			return nil
		}).
		Handle("content.special", func(_ context.Context, _ Message) error {
			handler = "exact"
			return nil
		})

	h := r.Handler()

	if err := h(context.Background(), Message{Topic: "content.special"}); err != nil {
		t.Fatal(err)
	}
	if handler != "exact" {
		t.Errorf("handler = %q, want exact", handler)
	}
}

func TestRouter_DefaultHandler(t *testing.T) {
	t.Parallel()

	var defaultCalled bool
	r := NewRouter().
		Handle("known", func(_ context.Context, _ Message) error {
			return nil
		}).
		Default(func(_ context.Context, _ Message) error {
			defaultCalled = true
			return nil
		})

	h := r.Handler()

	if err := h(context.Background(), Message{Topic: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if !defaultCalled {
		t.Error("default handler was not called")
	}
}

func TestRouter_NoMatchNoDefault(t *testing.T) {
	t.Parallel()

	r := NewRouter().
		Handle("known", func(_ context.Context, _ Message) error {
			t.Error("should not be called")
			return nil
		})

	h := r.Handler()
	if err := h(context.Background(), Message{Topic: "unknown"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouter_ErrorPropagation(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("handler error")
	r := NewRouter().
		Handle("fail", func(_ context.Context, _ Message) error {
			return sentinel
		})

	h := r.Handler()
	err := h(context.Background(), Message{Topic: "fail"})
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want %v", err, sentinel)
	}
}

func TestRouter_CustomKeyFunc(t *testing.T) {
	t.Parallel()

	var got string
	r := NewRouter(
		WithRouterKeyFunc(func(msg Message) string {
			return msg.Headers["event-type"]
		}),
	).Handle("user.created", func(_ context.Context, msg Message) error {
		got = msg.Headers["event-type"]
		return nil
	})

	h := r.Handler()
	msg := Message{
		Topic:   "events",
		Headers: map[string]string{"event-type": "user.created"},
	}
	if err := h(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if got != "user.created" {
		t.Errorf("got = %q, want user.created", got)
	}
}

func TestRouter_ConcurrentUsage(t *testing.T) {
	t.Parallel()

	var count atomic.Int64
	r := NewRouter().
		Handle("topic.*", func(_ context.Context, _ Message) error {
			count.Add(1)
			return nil
		})

	h := r.Handler()
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = h(context.Background(), Message{Topic: "topic.test"})
		}()
	}

	wg.Wait()
	if count.Load() != goroutines {
		t.Errorf("count = %d, want %d", count.Load(), goroutines)
	}
}
