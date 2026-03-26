package worker

import "time"

// EventType identifies the kind of event emitted by a handler.
type EventType int

const (
	EventProgress EventType = iota // Progress update (bytes, percent, message)
	EventPartial                   // Usable partial result before completion
	EventLog                       // Structured log from the handler
	EventResult                    // Final result (auto-emitted on success)
	EventError                     // Error (auto-emitted on failure)
)

// String returns a human-readable event type name.
func (t EventType) String() string {
	switch t {
	case EventProgress:
		return "progress"
	case EventPartial:
		return "partial"
	case EventLog:
		return "log"
	case EventResult:
		return "result"
	case EventError:
		return "error"
	default:
		return "unknown"
	}
}

// Event is a typed message emitted by a handler during execution.
type Event[O any] struct {
	Type      EventType      `json:"type"`
	TaskID    string         `json:"task_id"`
	WorkerID  string         `json:"worker_id"`
	Progress  *Progress      `json:"progress,omitempty"`
	Data      O              `json:"data,omitempty"`
	Error     error          `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Progress reports quantitative progress.
type Progress struct {
	Current int64   `json:"current"`           // e.g., bytes downloaded
	Total   int64   `json:"total"`             // total expected (-1 if unknown)
	Percent float64 `json:"percent,omitempty"` // 0.0–1.0 (auto-computed if Total > 0)
	Message string  `json:"message,omitempty"` // human-readable status
}

// ProgressEvent creates a progress event with the given current/total counts.
func ProgressEvent[O any](current, total int64, msg string) Event[O] {
	var pct float64
	if total > 0 {
		pct = float64(current) / float64(total)
	}
	return Event[O]{
		Type: EventProgress,
		Progress: &Progress{
			Current: current,
			Total:   total,
			Percent: pct,
			Message: msg,
		},
		Timestamp: time.Now(),
	}
}

// PartialEvent creates a partial-result event.
func PartialEvent[O any](data O) Event[O] {
	return Event[O]{
		Type:      EventPartial,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// LogEvent creates a log event with optional metadata.
func LogEvent[O any](msg string, meta map[string]any) Event[O] {
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["message"] = msg
	return Event[O]{
		Type:      EventLog,
		Metadata:  meta,
		Timestamp: time.Now(),
	}
}

// resultEvent creates an internal result event (auto-emitted by pool on success).
func resultEvent[O any](data O) Event[O] {
	return Event[O]{
		Type:      EventResult,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// errorEvent creates an internal error event (auto-emitted by pool on failure).
func errorEvent[O any](err error) Event[O] {
	return Event[O]{
		Type:      EventError,
		Error:     err,
		Timestamp: time.Now(),
	}
}
