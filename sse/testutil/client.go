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
	// ID is the `id:` field used for Last-Event-ID reconnection, if present.
	ID string
}

// StreamClient reads and decodes framed SSE events from an open response body.
// It is not safe for concurrent use; drive it from a single test goroutine.
type StreamClient struct {
	resp   *http.Response
	reader *bufio.Reader
}

// newStreamClient wraps an open SSE response body for frame-by-frame reading.
func newStreamClient(resp *http.Response) *StreamClient {
	return &StreamClient{resp: resp, reader: bufio.NewReader(resp.Body)}
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
func (c *StreamClient) Next() (Event, error) {
	var (
		evt      Event
		data     []string
		haveData bool
	)
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			// A partial trailing frame (no terminating blank line) is malformed;
			// discard whatever was accumulated and surface the stream error.
			if err == io.EOF {
				return Event{}, io.EOF
			}
			return Event{}, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// Frame boundary: emit only if we accumulated real fields.
			if haveData || evt.Name != "" || evt.ID != "" {
				evt.Data = []byte(strings.Join(data, "\n"))
				return evt, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / keep-alive
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			evt.Name = value
		case "data":
			data = append(data, value)
			haveData = true
		case "id":
			evt.ID = value
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
