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

	mu     sync.Mutex
	result O
	err    error
	closed bool
}

func newTaskHandle[O any](cancel context.CancelFunc, eventBuffer int) *TaskHandle[O] {
	return &TaskHandle[O]{
		id:     uuid.NewString(),
		events: make(chan Event[O], eventBuffer),
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

// ID returns the unique task identifier.
func (h *TaskHandle[O]) ID() string { return h.id }

// Events returns a channel of events for this task. Closed when task completes.
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

// emit sends an event to the task's event channel. Safe to call concurrently; no-op after complete.
// The lock is held during the channel send to prevent a TOCTOU race with complete() closing the channel.
// The events channel is buffered, so this should not block under normal usage.
// Handlers must not call emit after returning from Handle().
func (h *TaskHandle[O]) emit(e Event[O]) {
	e.TaskID = h.id

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.events <- e
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
