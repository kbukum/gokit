package worker

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// TaskHandle tracks a submitted task's lifecycle.
type TaskHandle[O any] struct {
	id     string
	events chan Event[O]
	done   chan struct{}
	cancel context.CancelFunc
	ctx    context.Context

	mu     sync.Mutex
	result O
	err    error
	closed bool
}

func newTaskHandle[O any](ctx context.Context, cancel context.CancelFunc, eventBuffer int) *TaskHandle[O] {
	return &TaskHandle[O]{
		id:     uuid.NewString(),
		events: make(chan Event[O], eventBuffer),
		done:   make(chan struct{}),
		cancel: cancel,
		ctx:    ctx,
	}
}

// ID returns the unique task identifier.
func (h *TaskHandle[O]) ID() string { return h.id }

// Events returns the bounded task stream. While active, producers block when it is full. A consumer that stops reading must cancel the task; cancellation releases blocked sends. Result remains authoritative when canceled events cannot be delivered.
func (h *TaskHandle[O]) Events() <-chan Event[O] { return h.events }

// Done returns a channel that is closed when the task completes.
func (h *TaskHandle[O]) Done() <-chan struct{} { return h.done }

// Cancel requests cancellation of this specific task.
func (h *TaskHandle[O]) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

// Result blocks until the task completes and returns the final result.
func (h *TaskHandle[O]) Result() (O, error) {
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.result, h.err
}

// emit serializes send and close. Active tasks apply bounded backpressure; cancellation makes a full queue non-blocking. A terminal event can still enter an available slot after cancellation.
func (h *TaskHandle[O]) emit(e Event[O]) {
	e.TaskID = h.id

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	select {
	case h.events <- e:
		return
	default:
	}
	select {
	case h.events <- e:
	case <-h.ctx.Done():
	}
}

// complete finalizes the task handle.
func (h *TaskHandle[O]) complete(result O, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.result = result
	h.err = err
	h.closed = true
	close(h.events)
	close(h.done)
}
