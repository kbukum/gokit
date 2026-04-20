package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/sse"
)

// SSEBridgeOption configures an SSEBridge.
type SSEBridgeOption func(*sseBridgeConfig)

type sseBridgeConfig struct {
	topicFunc func(event Event[any]) string
}

// WithTopicFunc sets a custom function to derive the SSE broadcast pattern
// from a worker event. Defaults to "task:{taskID}".
func WithTopicFunc(fn func(event Event[any]) string) SSEBridgeOption {
	return func(cfg *sseBridgeConfig) {
		cfg.topicFunc = fn
	}
}

// SSEBridge connects a worker pool's events to an SSE broadcaster for
// real-time progress streaming.
type SSEBridge[I, O any] struct {
	pool        *Pool[I, O]
	broadcaster sse.Broadcaster
	cfg         sseBridgeConfig
}

// NewSSEBridge creates a bridge that forwards pool events to an SSE broadcaster.
func NewSSEBridge[I, O any](pool *Pool[I, O], broadcaster sse.Broadcaster, opts ...SSEBridgeOption) *SSEBridge[I, O] {
	cfg := sseBridgeConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &SSEBridge[I, O]{
		pool:        pool,
		broadcaster: broadcaster,
		cfg:         cfg,
	}
}

// sseEvent is the JSON envelope sent to SSE clients.
type sseEvent struct {
	Type     string `json:"type"`
	TaskID   string `json:"task_id"`
	WorkerID string `json:"worker_id"`
	Data     any    `json:"data,omitempty"`
	Progress any    `json:"progress,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Start begins forwarding pool events to SSE clients.
// Returns a stop function that terminates the bridge goroutine.
func (b *SSEBridge[I, O]) Start(ctx context.Context) (stop func()) {
	bridgeCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		b.forward(bridgeCtx)
	}()

	return func() {
		cancel()
		wg.Wait()
	}
}

func (b *SSEBridge[I, O]) forward(ctx context.Context) {
	events := b.pool.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			b.broadcastEvent(ev)
		}
	}
}

func (b *SSEBridge[I, O]) broadcastEvent(ev Event[O]) {
	se := sseEvent{
		Type:     ev.Type.String(),
		TaskID:   ev.TaskID,
		WorkerID: ev.WorkerID,
	}

	if ev.Progress != nil {
		se.Progress = ev.Progress
	}
	if ev.Error != nil {
		se.Error = ev.Error.Error()
	}

	// Marshal data — ignore errors for non-serializable types.
	se.Data = ev.Data

	data, err := json.Marshal(se)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"type":"error","task_id":%q,"error":"marshal failed"}`, ev.TaskID))
	}

	topic := b.topic(ev)
	b.broadcaster.BroadcastToPattern(topic, data)
}

func (b *SSEBridge[I, O]) topic(ev Event[O]) string {
	if b.cfg.topicFunc != nil {
		// Convert to Event[any] for the topic function.
		generic := Event[any]{
			Type:      ev.Type,
			TaskID:    ev.TaskID,
			WorkerID:  ev.WorkerID,
			Progress:  ev.Progress,
			Timestamp: ev.Timestamp,
			Metadata:  ev.Metadata,
		}
		return b.cfg.topicFunc(generic)
	}
	return fmt.Sprintf("task:%s", ev.TaskID)
}
