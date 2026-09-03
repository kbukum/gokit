package sse

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// drainDecoder reads every dispatchable event until EOF. Any non-EOF read error
// fails the test.
func drainDecoder(t *testing.T, d *Decoder) []DecodedEvent {
	t.Helper()
	var events []DecodedEvent
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		events = append(events, ev)
	}
}

func TestDecoder_Next_Framing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []DecodedEvent
	}{
		{
			name: "single data frame",
			raw:  "data: hello\n\n",
			want: []DecodedEvent{{Data: []byte("hello")}},
		},
		{
			name: "named event",
			raw:  "event: ping\ndata: {}\n\n",
			want: []DecodedEvent{{Event: "ping", Data: []byte("{}")}},
		},
		{
			name: "multiline data is newline-joined",
			raw:  "data: line1\ndata: line2\ndata: line3\n\n",
			want: []DecodedEvent{{Data: []byte("line1\nline2\nline3")}},
		},
		{
			name: "comment and keepalive lines are skipped",
			raw:  ": keepalive 123\ndata: after-comment\n\n",
			want: []DecodedEvent{{Data: []byte("after-comment")}},
		},
		{
			name: "CRLF line endings",
			raw:  "event: ping\r\ndata: crlf\r\n\r\n",
			want: []DecodedEvent{{Event: "ping", Data: []byte("crlf")}},
		},
		{
			name: "CR-only line endings",
			raw:  "event: ping\rdata: cr\r\r",
			want: []DecodedEvent{{Event: "ping", Data: []byte("cr")}},
		},
		{
			name: "leading UTF-8 BOM is ignored",
			raw:  "\ufeffdata: bom\n\n",
			want: []DecodedEvent{{Data: []byte("bom")}},
		},
		{
			name: "only the first BOM is stripped",
			raw:  "\ufeffdata: \ufeffkept\n\n",
			want: []DecodedEvent{{Data: []byte("\ufeffkept")}},
		},
		{
			name: "unknown fields are ignored",
			raw:  "retry: 5000\nfoo: bar\ndata: kept\n\n",
			want: []DecodedEvent{{Data: []byte("kept")}},
		},
		{
			name: "event-only block is not dispatched",
			raw:  "event: lonely\n\ndata: real\n\n",
			want: []DecodedEvent{{Data: []byte("real")}},
		},
		{
			name: "id-only block updates persistent id without dispatching",
			raw:  "id: 42\n\nevent: ping\ndata: x\n\n",
			want: []DecodedEvent{{Event: "ping", Data: []byte("x"), ID: "42"}},
		},
		{
			name: "last-event id persists across events until changed",
			raw:  "id: 1\ndata: a\n\ndata: b\n\nid: 2\ndata: c\n\n",
			want: []DecodedEvent{
				{Data: []byte("a"), ID: "1"},
				{Data: []byte("b"), ID: "1"},
				{Data: []byte("c"), ID: "2"},
			},
		},
		{
			name: "id containing NUL is ignored and previous id is kept",
			raw:  "id: 1\ndata: a\n\nid: x\x00y\ndata: b\n\n",
			want: []DecodedEvent{
				{Data: []byte("a"), ID: "1"},
				{Data: []byte("b"), ID: "1"},
			},
		},
		{
			name: "truncated trailing frame is discarded at EOF",
			raw:  "data: complete\n\ndata: truncated\n",
			want: []DecodedEvent{{Data: []byte("complete")}},
		},
		{
			name: "empty data field yields an empty payload",
			raw:  "data:\n\n",
			want: []DecodedEvent{{Data: []byte("")}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := drainDecoder(t, NewDecoder(strings.NewReader(tt.raw)))
			if len(got) != len(tt.want) {
				t.Fatalf("got %d events, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				if got[i].Event != w.Event {
					t.Errorf("event %d: name = %q, want %q", i, got[i].Event, w.Event)
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

// TestDecoder_Next_LargePayload covers a data line well above the 64 KiB ceiling
// of the default bufio.Scanner, which the bounded DefaultMaxLineSize accommodates.
func TestDecoder_Next_LargePayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("x", 256*1024) // 256 KiB, > 64 KiB
	got := drainDecoder(t, NewDecoder(strings.NewReader("data: "+payload+"\n\n")))
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if string(got[0].Data) != payload {
		t.Errorf("large payload not decoded intact: got %d bytes, want %d", len(got[0].Data), len(payload))
	}
}

// TestDecoder_Next_LineExceedsMax verifies a line beyond the configured maximum
// surfaces the scanner error rather than silently truncating.
func TestDecoder_Next_LineExceedsMax(t *testing.T) {
	t.Parallel()

	d := NewDecoder(strings.NewReader("data: "+strings.Repeat("x", 16*1024)+"\n\n"), WithMaxLineSize(1024))
	_, err := d.Next()
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected a scanner error for an over-long line, got %v", err)
	}
}

// FuzzDecoder_Next ensures the decoder never panics and always terminates on
// arbitrary input, returning only io.EOF or nil read errors.
func FuzzDecoder_Next(f *testing.F) {
	seeds := []string{
		"data: hello\n\n",
		"event: ping\r\ndata: x\r\n\r\n",
		": comment\nid: 1\ndata: a\ndata: b\n\n",
		"id: 7\n\n",
		"data: truncated\n",
		"\ufeffdata: bom\n\n",
		"",
		"\n\n\n",
		"data",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		d := NewDecoder(strings.NewReader(raw))
		for range 1024 { // bound iterations so malformed input can never loop forever
			_, err := d.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("unexpected non-EOF error: %v", err)
				}
				return
			}
		}
	})
}
