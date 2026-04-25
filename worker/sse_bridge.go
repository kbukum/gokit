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
	topicFunc    func(event Event[any]) string
	envelopeFunc func(event Event[any]) any
}

// WithTopicFunc sets a custom function to derive the SSE broadcast pattern
// from a worker event. Defaults to "task:{taskID}".
func WithTopicFunc(fn func(event Event[any]) string) SSEBridgeOption {
	return func(cfg *sseBridgeConfig) {
		cfg.topicFunc = fn
	}
}

// WithEnvelope replaces the default JSON event payload with one returned by
// fn. Use this to project worker events into a domain-specific schema (e.g.
// adding workspace_id, attempt_id, ts) without modifying the bridge.
//
// The function receives the event projected to Event[any] so it can be
// shared across input/output type parameters. The returned value is JSON
// marshaled directly — return any serializable type.
func WithEnvelope(fn func(event Event[any]) any) SSEBridgeOption {
	return func(cfg *sseBridgeConfig) {
		cfg.envelopeFunc = fn
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
	var payload any
	if b.cfg.envelopeFunc != nil {
		payload = b.cfg.envelopeFunc(toGenericEvent(ev))
	} else {
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
		se.Data = ev.Data
		payload = se
	}

	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"type":"error","task_id":%q,"error":"marshal failed"}`, ev.TaskID))
	}

	topic := b.topic(ev)
	b.broadcaster.BroadcastToPattern(topic, data)
}

func (b *SSEBridge[I, O]) topic(ev Event[O]) string {
	if b.cfg.topicFunc != nil {
		return b.cfg.topicFunc(toGenericEvent(ev))
	}
	return fmt.Sprintf("task:%s", ev.TaskID)
}

// toGenericEvent projects a typed worker event onto Event[any] so callbacks
// can be type-erased without losing the data payload.
func toGenericEvent[O any](ev Event[O]) Event[any] {
	return Event[any]{
		Type:      ev.Type,
		TaskID:    ev.TaskID,
		WorkerID:  ev.WorkerID,
		Data:      ev.Data,
		Progress:  ev.Progress,
		Error:     ev.Error,
		Timestamp: ev.Timestamp,
		Metadata:  ev.Metadata,
	}
}
