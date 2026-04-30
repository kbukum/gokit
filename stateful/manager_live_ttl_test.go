package stateful

import (
	"context"
	"testing"
	"time"
)

func TestManager_LiveTTLCleanup(t *testing.T) {
	t.Parallel()

	expired := make(chan string, 1)
	mgr := NewManager(
		func(key string) *Accumulator[int] {
			return NewAccumulator(NewMemoryStore[int](), Config[int]{
				TTL: 30 * time.Millisecond,
				OnExpire: func(_ context.Context, key string) {
					expired <- key
				},
			})
		},
		30*time.Millisecond,
	)
	defer mgr.Close()

	if err := mgr.Append(context.Background(), "session", 1); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	select {
	case key := <-expired:
		if key != "session" {
			t.Fatalf("expired key = %q, want session", key)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected background TTL cleanup to expire accumulator")
	}

	if acc := mgr.Get("session"); acc != nil {
		t.Fatal("expected accumulator to be removed by background cleanup")
	}
}
