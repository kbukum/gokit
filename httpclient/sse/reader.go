// Package sse provides a reusable Server-Sent Events reader.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// Event represents a single server-sent event.
type Event struct {
	// Event is the SSE event type (from "event:" line). Empty for data-only events.
	Event string
	// Data is the event payload (from "data:" line(s)). Multi-line data is joined with newlines.
	Data string
	// ID is the event ID (from "id:" line).
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
	scanner *bufio.Scanner
	body    io.ReadCloser
}

// NewReader creates an SSE reader from a readable stream.
func NewReader(body io.ReadCloser) Reader {
	return &reader{
		scanner: bufio.NewScanner(body),
		body:    body,
	}
}

// Next returns the next SSE event. Returns io.EOF when the stream ends.
func (r *reader) Next() (*Event, error) {
	var event Event
	var hasData bool

	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Blank line signals end of event
		if line == "" {
			if hasData {
				return &event, nil
			}
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		field, value := parseSSELine(line)
		switch field {
		case "data":
			if hasData {
				event.Data += "\n" + value
			} else {
				event.Data = value
				hasData = true
			}
		case "event":
			event.Event = value
		case "id":
			event.ID = value
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	// Stream ended — return last event if present
	if hasData {
		return &event, nil
	}
	return nil, io.EOF
}

// Close releases the underlying stream.
func (r *reader) Close() error {
	return r.body.Close()
}

// parseSSELine parses a single SSE line into field and value.
func parseSSELine(line string) (field, value string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field = line[:idx]
	value = line[idx+1:]
	// Strip single leading space after colon per SSE spec
	if value != "" && value[0] == ' ' {
		value = value[1:]
	}
	return field, value
}
