package worker

import "errors"

// OverflowPolicy controls what happens when a worker queue is full.
type OverflowPolicy int

const (
	// Block waits until queue capacity is available.
	Block OverflowPolicy = iota
	// Reject fails the submission immediately.
	Reject
	// DropOldest evicts the oldest queued task to make room.
	DropOldest
)

var (
	// ErrQueueFull is returned when a task cannot be enqueued immediately.
	ErrQueueFull = errors.New("worker: queue full")
	// ErrTaskDropped is reported to a task that was evicted by DropOldest.
	ErrTaskDropped = errors.New("worker: task dropped due to overflow")
)
