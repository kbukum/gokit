package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/httpclient"
)

func collectStreamEvents(ch <-chan StreamEvent) (string, bool) {
	var content strings.Builder
	var gotErr bool
	for event := range ch {
		switch e := event.(type) {
		case TextDelta:
			content.WriteString(e.Text)
		case StreamError:
			gotErr = true
		}
	}
	return content.String(), gotErr
}

func TestStream_NDJSON_EmptyLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		f := w.(http.Flusher)
		lines := []string{
			`{"content":"A","done":false}`,
			`{"content":"B","done":false}`,
			`{"content":"","done":true}`,
		}
		for _, l := range lines {
			fmt.Fprintln(w, l)
			f.Flush()
		}
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	content, gotErr := collectStreamEvents(ch)
	if gotErr {
		t.Fatal("expected no stream error")
	}
	if got := content; got != "AB" {
		t.Errorf("content = %q, want %q", got, "AB")
	}
}

func TestStream_NDJSON_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		f := w.(http.Flusher)
		fmt.Fprintln(w, `{"content":"ok","done":false}`)
		f.Flush()
		fmt.Fprintln(w, `{not-valid-json}`)
		f.Flush()
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	_, gotErr := collectStreamEvents(ch)
	if !gotErr {
		t.Error("expected parse error for malformed JSON, got none")
	}
}

func TestStream_NDJSON_LargeChunk(t *testing.T) {
	largeContent := strings.Repeat("x", 48*1024) // 48 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		payload := map[string]any{"content": largeContent, "done": true}
		raw, _ := json.Marshal(payload)
		fmt.Fprintln(w, string(raw))
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	content, gotErr := collectStreamEvents(ch)
	if gotErr {
		t.Fatal("expected no stream error")
	}
	if len(content) != len(largeContent) {
		t.Errorf("content length = %d, want %d", len(content), len(largeContent))
	}
}

func TestStream_SSE_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		io.WriteString(w, "data: {INVALID_JSON}\n\n")
		f.Flush()
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamSSE}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	_, gotErr := collectStreamEvents(ch)
	if !gotErr {
		t.Error("expected parse error for malformed SSE payload, got none")
	}
}

func TestStream_NDJSON_NilBody(t *testing.T) {
	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch := make(chan streamChunk, 1)
	go a.readNDJSONStream(context.Background(), nil, ch)

	chunk := <-ch
	if !errors.Is(chunk.Err, ErrNoStreamBody) {
		t.Errorf("expected ErrNoStreamBody, got %v", chunk.Err)
	}
}

func TestStream_SSE_NilReader(t *testing.T) {
	d := &mockDialect{streamFormat: StreamSSE}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch := make(chan streamChunk, 1)
	go a.readSSEStream(context.Background(), nil, ch)

	chunk := <-ch
	if !errors.Is(chunk.Err, ErrNoSSEReader) {
		t.Errorf("expected ErrNoSSEReader, got %v", chunk.Err)
	}
}

func TestStream_UnsupportedFormat_CtxCancelDoesNotBlock(t *testing.T) {
	d := &mockDialect{streamFormat: StreamFormat(99)}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan streamChunk) // unbuffered, no consumer: an unguarded send would deadlock
	done := make(chan struct{})
	go func() {
		a.readStream(ctx, &httpclient.StreamResponse{}, ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readStream blocked on an abandoned channel after context cancel")
	}
	if _, open := <-ch; open {
		t.Fatal("expected channel to be closed without an emitted chunk")
	}
}

func TestStream_NDJSON_NilBody_CtxCancelDoesNotBlock(t *testing.T) {
	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan streamChunk) // unbuffered, no consumer
	done := make(chan struct{})
	go func() {
		a.readNDJSONStream(ctx, nil, ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readNDJSONStream blocked on an abandoned channel after context cancel")
	}
}

func TestStream_ContextCancelDuringNDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		f := w.(http.Flusher)
		fmt.Fprintln(w, `{"content":"first","done":false}`)
		f.Flush()
		time.Sleep(2 * time.Second)
		fmt.Fprintln(w, `{"content":"never","done":true}`)
		f.Flush()
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Stream(ctx, CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	first := <-ch
	content, ok := first.(TextDelta)
	if !ok {
		t.Fatalf("first event = %T, want TextDelta", first)
	}
	if content.Text != "first" {
		t.Errorf("first content = %q, want %q", content.Text, "first")
	}
	cancel()

	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timer.C:
			t.Fatal("channel not closed within 1s after context cancel")
		}
	}
}

func TestStreamChunk_DoneWithContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"content":"final","done":true}`)
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	content, gotErr := collectStreamEvents(ch)
	if gotErr {
		t.Fatal("expected no stream error")
	}
	if content != "final" {
		t.Errorf("content = %q, want %q", content, "final")
	}
}
