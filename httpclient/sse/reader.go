// Package sse provides a reusable Server-Sent Events reader.
package sse

import (
	"io"

	rootsse "github.com/kbukum/gokit/sse"
)

// Event represents a single server-sent event.
type Event struct {
	// Event is the SSE event type (from "event:" line). Empty for data-only events.
	Event string
	// Data is the event payload (from "data:" line(s)). Multi-line data is joined with newlines.
	Data string
	// ID is the persistent last-event ID in effect for this event: set by the most
	// recent "id:" line and carried forward across events until changed.
	ID string
}

// Reader reads server-sent events from a stream.
type Reader interface {
	// Next returns the next SSE event. Returns io.EOF when the stream ends.
	Next() (*Event, error)
	// Close releases the underlying resources.
	Close() error
}

type reader struct {
	dec  *rootsse.Decoder
	body io.ReadCloser
}

// NewReader creates an SSE reader from a readable stream. Decoding is delegated
// to the canonical [rootsse.Decoder], so this reader and the SSE test harness
// share one spec-correct parser (CR/LF/CRLF line endings, a single leading BOM
// ignored, comment lines skipped, NUL ids ignored, and a truncated trailing
// block discarded at EOF rather than surfaced as a partial event).
func NewReader(body io.ReadCloser) Reader {
	return &reader{dec: rootsse.NewDecoder(body), body: body}
}

// Next returns the next SSE event. Returns io.EOF when the stream ends.
func (r *reader) Next() (*Event, error) {
	ev, err := r.dec.Next()
	if err != nil {
		return nil, err
	}
	return &Event{Event: ev.Event, Data: string(ev.Data), ID: ev.ID}, nil
}

// Close releases the underlying stream.
func (r *reader) Close() error {
	return r.body.Close()
}
