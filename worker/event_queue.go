package worker

import "sync"

// eventQueue reserves one control slot so overflow is observable even without a reader.
type eventQueue[O any] struct {
	mu     sync.Mutex
	events chan Event[O]
	limit  int
	closed bool
}

func newEventQueue[O any](limit int) *eventQueue[O] {
	return &eventQueue[O]{events: make(chan Event[O], limit+1), limit: limit}
}

func (q *eventQueue[O]) emit(event Event[O]) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	if len(q.events) >= q.limit {
		q.events <- Event[O]{Type: EventError, TaskID: event.TaskID, WorkerID: event.WorkerID, Error: ErrEventOverflow}
		q.closed = true
		close(q.events)
		return
	}
	q.events <- event
}

func (q *eventQueue[O]) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.events)
	}
}
