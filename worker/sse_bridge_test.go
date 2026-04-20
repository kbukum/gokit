package worker_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

// mockBroadcaster captures SSE broadcasts for testing.
type mockBroadcaster struct {
	mu       sync.Mutex
	messages []broadcastMessage
}

type broadcastMessage struct {
	Pattern string
	Data    []byte
}

func (m *mockBroadcaster) BroadcastToPattern(pattern string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, broadcastMessage{Pattern: pattern, Data: data})
}

func (m *mockBroadcaster) getMessages() []broadcastMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]broadcastMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

func TestSSEBridge_ForwardsEvents(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		emit(worker.ProgressEvent[string](50, 100, "halfway"))
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "sse-test", Size: 1, EventBuffer: 16})
	bc := &mockBroadcaster{}
	bridge := worker.NewSSEBridge(pool, bc)

	ctx := context.Background()
	stop := bridge.Start(ctx)

	handle, err := pool.Submit(ctx, "hello")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Wait for task completion.
	_, _ = handle.Result()

	// Give the bridge goroutine time to process events.
	time.Sleep(50 * time.Millisecond)

	stop()
	_ = pool.Stop(ctx)

	msgs := bc.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected at least one SSE message")
	}

	// Verify the messages are valid JSON with expected fields.
	for _, msg := range msgs {
		var parsed map[string]any
		if err := json.Unmarshal(msg.Data, &parsed); err != nil {
			t.Errorf("invalid JSON: %v", err)
			continue
		}
		if _, ok := parsed["type"]; !ok {
			t.Error("SSE message missing 'type' field")
		}
		if _, ok := parsed["task_id"]; !ok {
			t.Error("SSE message missing 'task_id' field")
		}
	}
}

func TestSSEBridge_TopicPattern(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "topic-test", Size: 1, EventBuffer: 16})
	bc := &mockBroadcaster{}
	bridge := worker.NewSSEBridge(pool, bc)

	ctx := context.Background()
	stop := bridge.Start(ctx)

	handle, err := pool.Submit(ctx, "test-input")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	_, _ = handle.Result()
	time.Sleep(50 * time.Millisecond)

	stop()
	_ = pool.Stop(ctx)

	msgs := bc.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}

	// Default topic pattern should be "task:{taskID}".
	taskID := handle.ID()
	expectedPattern := "task:" + taskID
	for _, msg := range msgs {
		if msg.Pattern != expectedPattern {
			t.Errorf("pattern = %q, want %q", msg.Pattern, expectedPattern)
		}
	}
}

func TestSSEBridge_StopCleansUp(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "stop-test", Size: 1, EventBuffer: 16})
	bc := &mockBroadcaster{}
	bridge := worker.NewSSEBridge(pool, bc)

	ctx := context.Background()
	stop := bridge.Start(ctx)

	// Stop should return without hanging.
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return within timeout")
	}

	_ = pool.Stop(ctx)
}

func TestSSEBridge_ContextCancellation(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "ctx-cancel", Size: 1, EventBuffer: 16})
	bc := &mockBroadcaster{}
	bridge := worker.NewSSEBridge(pool, bc)

	ctx, cancel := context.WithCancel(context.Background())
	stop := bridge.Start(ctx)

	// Cancel the context — bridge should stop.
	cancel()

	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return after context cancellation")
	}

	_ = pool.Stop(context.Background())
}
