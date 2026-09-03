package sse

import (
	"bufio"
	"io"
	"strings"
)

// DefaultMaxLineSize bounds the number of bytes the [Decoder] buffers for a
// single event-stream line. It defaults to 1 MiB so a large `data:` payload is
// decoded without the 64 KiB ceiling of the default [bufio.Scanner]; override it
// with [WithMaxLineSize] when a stream carries larger lines.
const DefaultMaxLineSize = 1 << 20

// DecodedEvent is a single event decoded from an SSE byte stream.
//
// Data is the concatenated `data:` fields joined by newlines with no trailing
// newline. ID is the persistent last-event ID in effect for this event: it is
// set by the most recent valid `id:` field and carried forward across blocks
// until changed, matching reconnection semantics.
type DecodedEvent struct {
	// Event is the SSE event type (the `event:` field), empty for the default
	// `message` event.
	Event string
	// Data is the concatenated payload with the trailing newline removed.
	Data []byte
	// ID is the persistent last-event ID in effect for this event.
	ID string
}

// Decoder reads and decodes framed SSE events from a byte stream following the
// WHATWG event-stream dispatch algorithm: lines end on CR, LF, or CRLF; a single
// leading UTF-8 BOM is ignored; comment (`:`) lines are skipped; an `id:` value
// containing U+0000 is ignored; and only a block that carried a `data:` field is
// dispatched, so an `id`-only or `event`-only block updates state without
// surfacing a spurious event. A block is complete only at its blank-line
// boundary — if the stream ends mid-block, the pending fields are discarded and
// [io.EOF] is returned rather than a truncated event.
//
// A Decoder is not safe for concurrent use; drive it from a single goroutine.
type Decoder struct {
	scanner     *bufio.Scanner
	lastEventID string
	atStart     bool
}

// DecoderOption configures a [Decoder].
type DecoderOption func(*decoderConfig)

type decoderConfig struct {
	maxLineSize int
}

// WithMaxLineSize sets the maximum number of bytes buffered for a single
// event-stream line. A non-positive value is ignored, keeping
// [DefaultMaxLineSize].
func WithMaxLineSize(n int) DecoderOption {
	return func(c *decoderConfig) {
		if n > 0 {
			c.maxLineSize = n
		}
	}
}

// NewDecoder returns a [Decoder] reading framed SSE events from r.
func NewDecoder(r io.Reader, opts ...DecoderOption) *Decoder {
	cfg := decoderConfig{maxLineSize: DefaultMaxLineSize}
	for _, opt := range opts {
		opt(&cfg)
	}
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 4096), cfg.maxLineSize)
	s.Split(scanEventLines)
	return &Decoder{scanner: s, atStart: true}
}

// Next reads and returns the next complete SSE event, blocking until a blank-line
// frame boundary arrives. It returns [io.EOF] when the stream ends, discarding
// any partially-read trailing block rather than returning a truncated event.
func (d *Decoder) Next() (DecodedEvent, error) {
	var (
		name     string
		data     []string
		haveData bool
	)
	for {
		if !d.scanner.Scan() {
			if err := d.scanner.Err(); err != nil {
				return DecodedEvent{}, err
			}
			// The stream ended. A partial trailing block (no terminating blank
			// line) is malformed; discard it and report EOF.
			return DecodedEvent{}, io.EOF
		}
		line := d.scanner.Text()
		if d.atStart {
			// Ignore a single leading UTF-8 BOM at the very start of the stream so
			// the first field is not mis-parsed as "\ufeffdata".
			line = strings.TrimPrefix(line, "\ufeff")
			d.atStart = false
		}

		if line == "" {
			// Frame boundary. A block with no data field is not dispatched: an
			// id-only or event-only block only updates state (the persistent
			// last-event ID already applied below). Reset the per-block buffers and
			// keep reading for the next dispatchable event.
			if !haveData {
				name = ""
				data = data[:0]
				continue
			}
			return DecodedEvent{
				Event: name,
				Data:  []byte(strings.Join(data, "\n")),
				ID:    d.lastEventID,
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
			// The last-event ID persists across blocks and carries onto every later
			// dispatched event until changed. Per the SSE spec an id whose value
			// contains U+0000 is ignored, so the previous id is kept rather than
			// adopting an invalid reconnection token.
			if !strings.ContainsRune(value, '\x00') {
				d.lastEventID = value
			}
		}
	}
}

// scanEventLines is a [bufio.SplitFunc] that yields one line per call, stripping
// a trailing CR, LF, or CRLF. It defers on a lone trailing CR until more data (or
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
