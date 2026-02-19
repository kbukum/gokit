package sse

import (
	"io"
	"strings"
	"testing"
)

// mockReadCloser wraps a string reader as an io.ReadCloser.
type mockReadCloser struct {
	*strings.Reader
}

func (m *mockReadCloser) Close() error { return nil }

func newMockBody(s string) io.ReadCloser {
	return &mockReadCloser{strings.NewReader(s)}
}

func TestReader_SingleEvent(t *testing.T) {
	body := newMockBody("data: hello world\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "hello world" {
		t.Errorf("got data %q, want %q", ev.Data, "hello world")
	}
}

func TestReader_MultipleEvents(t *testing.T) {
	body := newMockBody("data: first\n\ndata: second\n\n")
	r := NewReader(body)
	defer r.Close()

	ev1, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev1.Data != "first" {
		t.Errorf("first event data = %q, want %q", ev1.Data, "first")
	}

	ev2, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev2.Data != "second" {
		t.Errorf("second event data = %q, want %q", ev2.Data, "second")
	}

	_, err = r.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReader_EventWithType(t *testing.T) {
	body := newMockBody("event: message\ndata: hello\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "message" {
		t.Errorf("event type = %q, want %q", ev.Event, "message")
	}
	if ev.Data != "hello" {
		t.Errorf("data = %q, want %q", ev.Data, "hello")
	}
}

func TestReader_EventWithID(t *testing.T) {
	body := newMockBody("id: 42\ndata: hello\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "42" {
		t.Errorf("id = %q, want %q", ev.ID, "42")
	}
}

func TestReader_MultiLineData(t *testing.T) {
	body := newMockBody("data: line1\ndata: line2\ndata: line3\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline2\nline3"
	if ev.Data != want {
		t.Errorf("data = %q, want %q", ev.Data, want)
	}
}

func TestReader_SkipsComments(t *testing.T) {
	body := newMockBody(": this is a comment\ndata: hello\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "hello" {
		t.Errorf("data = %q, want %q", ev.Data, "hello")
	}
}

func TestReader_EmptyStream(t *testing.T) {
	body := newMockBody("")
	r := NewReader(body)
	defer r.Close()

	_, err := r.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestReader_DataWithoutSpace(t *testing.T) {
	body := newMockBody("data:no-space\n\n")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "no-space" {
		t.Errorf("data = %q, want %q", ev.Data, "no-space")
	}
}

func TestReader_LastEventWithoutTrailingNewline(t *testing.T) {
	body := newMockBody("data: trailing")
	r := NewReader(body)
	defer r.Close()

	ev, err := r.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "trailing" {
		t.Errorf("data = %q, want %q", ev.Data, "trailing")
	}
}

func TestParseSSELine(t *testing.T) {
	tests := []struct {
		line  string
		field string
		value string
	}{
		{"data: hello", "data", "hello"},
		{"data:hello", "data", "hello"},
		{"event: msg", "event", "msg"},
		{"id: 1", "id", "1"},
		{"retry: 3000", "retry", "3000"},
		{"fieldonly", "fieldonly", ""},
	}
	for _, tt := range tests {
		f, v := parseSSELine(tt.line)
		if f != tt.field || v != tt.value {
			t.Errorf("parseSSELine(%q) = (%q, %q), want (%q, %q)", tt.line, f, v, tt.field, tt.value)
		}
	}
}
