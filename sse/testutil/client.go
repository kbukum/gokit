package testutil

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kbukum/gokit/sse"
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

// StreamClient reads and decodes framed SSE events from an open response body
// using the canonical [sse.Decoder], so the harness and production SSE clients
// share one spec-correct parser. It is not safe for concurrent use; drive it
// from a single test goroutine.
type StreamClient struct {
	resp *http.Response
	dec  *sse.Decoder
}

// newStreamClient wraps an open SSE response body for frame-by-frame reading.
func newStreamClient(resp *http.Response) *StreamClient {
	return &StreamClient{resp: resp, dec: sse.NewDecoder(resp.Body)}
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
// boundary arrives. Comment (`:`) lines — including keep-alives — are skipped,
// and a truncated trailing block is discarded with [io.EOF] rather than surfaced
// as a partial event. See [sse.Decoder] for the full framing contract.
func (c *StreamClient) Next() (Event, error) {
	ev, err := c.dec.Next()
	if err != nil {
		return Event{}, err
	}
	return Event{Name: ev.Event, Data: ev.Data, ID: ev.ID}, nil
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
