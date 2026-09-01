package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

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

// MarshalJSON encodes the event type as its snake_case wire string (e.g. "progress"). A
// string discriminant is reorder-safe and shared byte-for-byte across kits, unlike the
// underlying integer index.
func (t EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON decodes the snake_case wire string produced by MarshalJSON back into an
// EventType, rejecting unknown discriminants.
func (t *EventType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "progress":
		*t = EventProgress
	case "partial":
		*t = EventPartial
	case "log":
		*t = EventLog
	case "result":
		*t = EventResult
	case "error":
		*t = EventError
	default:
		return fmt.Errorf("worker: unknown event type %q", s)
	}
	return nil
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

// eventWire is the cross-kit JSON shape of an Event. Error is carried as a message string
// because the error interface has no stable wire form (a bare errors.New value encodes as
// {} and cannot be decoded back), so error events would otherwise not round-trip.
type eventWire[O any] struct {
	Type      EventType      `json:"type"`
	TaskID    string         `json:"task_id"`
	WorkerID  string         `json:"worker_id"`
	Progress  *Progress      `json:"progress,omitempty"`
	Data      O              `json:"data,omitempty"`
	Error     string         `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// MarshalJSON encodes the event on the cross-kit wire form, mapping any error to its message
// string so error events serialize losslessly and identically across kits.
func (e Event[O]) MarshalJSON() ([]byte, error) {
	w := eventWire[O]{
		Type:      e.Type,
		TaskID:    e.TaskID,
		WorkerID:  e.WorkerID,
		Progress:  e.Progress,
		Data:      e.Data,
		Timestamp: e.Timestamp,
		Metadata:  e.Metadata,
	}
	if e.Error != nil {
		w.Error = e.Error.Error()
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes the cross-kit wire form produced by MarshalJSON, reconstructing a
// non-empty error string as a plain error so error events round-trip.
func (e *Event[O]) UnmarshalJSON(data []byte) error {
	var w eventWire[O]
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.Type = w.Type
	e.TaskID = w.TaskID
	e.WorkerID = w.WorkerID
	e.Progress = w.Progress
	e.Data = w.Data
	e.Timestamp = w.Timestamp
	e.Metadata = w.Metadata
	if w.Error != "" {
		e.Error = errors.New(w.Error)
	} else {
		e.Error = nil
	}
	return nil
}

// Progress reports quantitative progress. Percent is on a 0–100 scale and is present only
// when Total is known; Total is omitted when the total unit count is unknown.
type Progress struct {
	Current int64    `json:"current"`           // e.g., bytes downloaded
	Total   *int64   `json:"total,omitempty"`   // total expected units; omitted if unknown
	Percent *float64 `json:"percent,omitempty"` // 0–100, present only when Total is known
	Message string   `json:"message,omitempty"` // human-readable status
}

// NewProgress builds a Progress value, computing Percent on a 0–100 scale when total is
// known. A total of zero reports 100 percent (nothing to do is complete); an unknown total
// (nil) leaves Percent unset.
func NewProgress(current int64, total *int64) Progress {
	p := Progress{Current: current, Total: total}
	if total != nil {
		pct := 100.0
		if *total != 0 {
			pct = float64(current) / float64(*total) * 100
		}
		p.Percent = &pct
	}
	return p
}

// ProgressEvent creates a progress event with the given current/total counts. A negative
// total means the total is unknown: total and percent are both omitted from the wire form.
func ProgressEvent[O any](current, total int64, msg string) Event[O] {
	var t *int64
	if total >= 0 {
		t = &total
	}
	p := NewProgress(current, t)
	p.Message = msg
	return Event[O]{
		Type:      EventProgress,
		Progress:  &p,
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
