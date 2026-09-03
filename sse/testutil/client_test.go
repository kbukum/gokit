package testutil

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/sse"
)

// parserFrom builds a StreamClient reading directly from raw SSE bytes, bypassing
// the HTTP transport so the wire parser can be exercised in isolation.
func parserFrom(raw string) *StreamClient {
	return &StreamClient{dec: sse.NewDecoder(strings.NewReader(raw))}
}

// drain reads every dispatchable event until EOF, returning them in order. Any
// non-EOF read error fails the test.
func drain(t *testing.T, c *StreamClient) []Event {
	t.Helper()
	var events []Event
	for {
		evt, err := c.Next()
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		events = append(events, evt)
	}
}

func TestStreamClient_Next_Framing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []Event
	}{
		{
			name: "single data frame",
			raw:  "data: hello\n\n",
			want: []Event{{Data: []byte("hello")}},
		},
		{
			name: "named event",
			raw:  "event: ping\ndata: {}\n\n",
			want: []Event{{Name: "ping", Data: []byte("{}")}},
		},
		{
			name: "multiline data is newline-joined",
			raw:  "data: line1\ndata: line2\ndata: line3\n\n",
			want: []Event{{Data: []byte("line1\nline2\nline3")}},
		},
		{
			name: "comment and keepalive lines are skipped",
			raw:  ": keepalive 123\ndata: after-comment\n\n",
			want: []Event{{Data: []byte("after-comment")}},
		},
		{
			name: "CRLF line endings",
			raw:  "event: ping\r\ndata: crlf\r\n\r\n",
			want: []Event{{Name: "ping", Data: []byte("crlf")}},
		},
		{
			name: "CR-only line endings",
			raw:  "event: ping\rdata: cr\r\r",
			want: []Event{{Name: "ping", Data: []byte("cr")}},
		},
		{
			name: "unknown fields are ignored",
			raw:  "retry: 5000\nfoo: bar\ndata: kept\n\n",
			want: []Event{{Data: []byte("kept")}},
		},
		{
			name: "event-only block is not dispatched",
			raw:  "event: lonely\n\ndata: real\n\n",
			want: []Event{{Data: []byte("real")}},
		},
		{
			name: "id-only block updates persistent id without dispatching",
			raw:  "id: 42\n\nevent: ping\ndata: x\n\n",
			want: []Event{{Name: "ping", Data: []byte("x"), ID: "42"}},
		},
		{
			name: "last-event id persists across events until changed",
			raw:  "id: 1\ndata: a\n\ndata: b\n\nid: 2\ndata: c\n\n",
			want: []Event{
				{Data: []byte("a"), ID: "1"},
				{Data: []byte("b"), ID: "1"},
				{Data: []byte("c"), ID: "2"},
			},
		},
		{
			name: "id containing NUL is ignored and previous id is kept",
			raw:  "id: 1\ndata: a\n\nid: x\x00y\ndata: b\n\n",
			want: []Event{
				{Data: []byte("a"), ID: "1"},
				{Data: []byte("b"), ID: "1"},
			},
		},
		{
			name: "truncated trailing frame is discarded at EOF",
			raw:  "data: complete\n\ndata: truncated\n",
			want: []Event{{Data: []byte("complete")}},
		},
		{
			name: "empty data field yields an empty payload",
			raw:  "data:\n\n",
			want: []Event{{Data: []byte("")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := drain(t, parserFrom(tt.raw))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d events, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if got[i].Name != w.Name {
					t.Errorf("event %d: name = %q, want %q", i, got[i].Name, w.Name)
				}
				if !bytes.Equal(got[i].Data, w.Data) {
					t.Errorf("event %d: data = %q, want %q", i, got[i].Data, w.Data)
				}
				if got[i].ID != w.ID {
					t.Errorf("event %d: id = %q, want %q", i, got[i].ID, w.ID)
				}
			}
		})
	}
}

// FuzzStreamClient_Next ensures the parser never panics and always terminates on
// arbitrary input, returning only io.EOF or nil read errors.
func FuzzStreamClient_Next(f *testing.F) {
	seeds := []string{
		"data: hello\n\n",
		"event: ping\r\ndata: x\r\n\r\n",
		": comment\nid: 1\ndata: a\ndata: b\n\n",
		"id: 7\n\n",
		"data: truncated\n",
		"",
		"\n\n\n",
		"data",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		c := parserFrom(raw)
		for range 1024 { // bound iterations so malformed input can never loop forever
			_, err := c.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("unexpected non-EOF error: %v", err)
				}
				return
			}
		}
	})
}
