package worker

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// TaskHandle tracks a submitted task's lifecycle.
type TaskHandle[O any] struct {
	id      string
	events  *eventQueue[O]
	done    chan struct{}
	cancel  context.CancelFunc
	release func()

	mu     sync.Mutex
	result O
	err    error
	closed bool
}

func newTaskHandle[O any](cancel context.CancelFunc, release func(), eventBuffer int) *TaskHandle[O] {
	return &TaskHandle[O]{
		id:      uuid.NewString(),
		events:  newEventQueue[O](eventBuffer),
		done:    make(chan struct{}),
		cancel:  cancel,
		release: release,
	}
}

// ID returns the unique task identifier.
func (h *TaskHandle[O]) ID() string { return h.id }

// Events returns the bounded task stream. Overflow emits EventError with ErrEventOverflow in a reserved control slot and closes the stream, without canceling the task. Consumers must handle this failure; Result remains authoritative.
func (h *TaskHandle[O]) Events() <-chan Event[O] { return h.events.events }

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

// emit never blocks task execution on an event reader.
func (h *TaskHandle[O]) emit(e Event[O]) {
	e.TaskID = h.id

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.events.emit(e)
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
	h.release()
	h.events.close()
	close(h.done)
}
