package testutil

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Event is a decoded SSE event read from a stream. Keep-alive comment lines are
// skipped by the reader and never surface as an Event.
type Event struct {
	// Name is the SSE event type (the `event:` field), or empty for the default
	// `message` event.
	Name string
	// Data is the concatenated `data:` payload with the trailing newline removed.
	Data []byte
	// ID is the persistent last-event ID in effect for this event (set by the
	// most recent `id:` field and carried forward across blocks), empty until an
	// `id:` field has been seen.
	ID string
}

// StreamClient reads and decodes framed SSE events from an open response body.
// It is not safe for concurrent use; drive it from a single test goroutine.
type StreamClient struct {
	resp        *http.Response
	scanner     *bufio.Scanner
	lastEventID string
}

// newStreamClient wraps an open SSE response body for frame-by-frame reading.
func newStreamClient(resp *http.Response) *StreamClient {
	return &StreamClient{resp: resp, scanner: newEventScanner(resp.Body)}
}

// newEventScanner builds a scanner that splits an SSE byte stream into lines on
// any of the three event-stream terminators (CR, LF, or CRLF), matching the SSE
// spec instead of bufio's LF-only [bufio.ScanLines].
func newEventScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Split(scanEventLines)
	return s
}

// scanEventLines is a [bufio.SplitFunc] that yields one line per call, stripping a
// trailing CR, LF, or CRLF. It defers on a lone trailing CR until more data (or
// EOF) reveals whether an LF follows, so CRLF is never split into two lines.
func scanEventLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 < len(data) {
				if data[i+1] == '\n' {
					return i + 2, data[:i], nil
				}
				return i + 1, data[:i], nil
			}
			if atEOF {
				return i + 1, data[:i], nil
			}
			return 0, nil, nil // lone trailing CR: wait for the byte after it
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Response returns the underlying HTTP response (headers, status, body).
func (c *StreamClient) Response() *http.Response { return c.resp }

// Close closes the underlying response body, ending the stream.
func (c *StreamClient) Close() error {
	if c.resp != nil && c.resp.Body != nil {
		return c.resp.Body.Close()
	}
	return nil
}

// Next reads the next complete SSE event, blocking until a blank-line frame
// boundary arrives. Comment (`:`) lines — including keep-alives — are skipped.
// A frame is only complete at its blank-line boundary: when the stream ends
// before that boundary, any partially-read fields are discarded and [io.EOF] is
// returned rather than a truncated event.
//
// Framing follows the SSE dispatch algorithm: only a block that carried a `data`
// field is dispatched, so an `id`-only or `event`-only block updates state
// without surfacing a spurious event. An `id` field sets a persistent
// last-event ID that carries onto every subsequent dispatched event (its
// [Event.ID]) until changed, matching reconnection semantics.
func (c *StreamClient) Next() (Event, error) {
	var (
		name     string
		data     []string
		haveData bool
	)
	for {
		if !c.scanner.Scan() {
			if err := c.scanner.Err(); err != nil {
				return Event{}, err
			}
			// The stream ended. A partial trailing frame (no terminating blank
			// line) is malformed; discard whatever was accumulated and report EOF
			// rather than a truncated event.
			return Event{}, io.EOF
		}
		line := c.scanner.Text()

		if line == "" {
			// Frame boundary. Per the SSE spec a block with no data field is not
			// dispatched: an id-only or event-only block only updates state (the
			// persistent last-event ID already applied below). Reset the per-block
			// buffers and keep reading for the next dispatchable event.
			if !haveData {
				name = ""
				data = data[:0]
				continue
			}
			return Event{
				Name: name,
				Data: []byte(strings.Join(data, "\n")),
				ID:   c.lastEventID,
			}, nil
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / keep-alive
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
		case "data":
			data = append(data, value)
			haveData = true
		case "id":
			// The last-event ID persists across blocks and carries onto every
			// later dispatched event until changed. Per the SSE spec an id whose
			// value contains U+0000 is ignored, so the previous id is kept rather
			// than adopting an invalid reconnection token.
			if !strings.ContainsRune(value, '\x00') {
				c.lastEventID = value
			}
		}
	}
}

// Require reads events until one named want arrives and returns it, failing the
// test on a read error, EOF, or a mismatched event before want. The connected
// handshake `connected` event is skipped automatically.
func (c *StreamClient) Require(t *testing.T, want string) Event {
	t.Helper()
	for {
		evt, err := c.Next()
		if err != nil {
			t.Fatalf("waiting for event %q: %v", want, err)
		}
		if evt.Name == "connected" && want != "connected" {
			continue
		}
		if evt.Name != want {
			t.Fatalf("expected event %q, got %q (data %q)", want, evt.Name, evt.Data)
		}
		return evt
	}
}

// RequireJSON reads until the want event arrives and decodes its data into v.
func (c *StreamClient) RequireJSON(t *testing.T, want string, v any) Event {
	t.Helper()
	evt := c.Require(t, want)
	if err := json.Unmarshal(evt.Data, v); err != nil {
		t.Fatalf("decoding event %q data %q: %v", want, evt.Data, err)
	}
	return evt
}

// SkipConnected consumes the initial `connected` handshake event and returns it.
func (c *StreamClient) SkipConnected(t *testing.T) Event {
	t.Helper()
	return c.Require(t, "connected")
}
