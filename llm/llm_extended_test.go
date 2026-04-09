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
)

// ---------------------------------------------------------------------------
// Extended mock dialect with configurable stream chunk parsing
// ---------------------------------------------------------------------------

type extMockDialect struct {
	mockDialect
	parseChunkFn func(data []byte) (string, bool, error)
}

func (d *extMockDialect) ParseStreamChunk(data []byte) (string, bool, error) {
	if d.parseChunkFn != nil {
		return d.parseChunkFn(data)
	}
	return d.mockDialect.ParseStreamChunk(data)
}

// ---------------------------------------------------------------------------
// Stream parsing edge cases
// ---------------------------------------------------------------------------

func TestStream_NDJSON_EmptyLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		f := w.(http.Flusher)
		lines := []string{
			"",
			`{"content":"A","done":false}`,
			"",
			"",
			`{"content":"B","done":false}`,
			"",
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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var content strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	if got := content.String(); got != "AB" {
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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var gotErr bool
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
		}
	}
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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var content strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
	}
	if content.Len() != len(largeContent) {
		t.Errorf("content length = %d, want %d", content.Len(), len(largeContent))
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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var gotErr bool
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
		}
	}
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

	ch := make(chan StreamChunk, 1)
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

	ch := make(chan StreamChunk, 1)
	go a.readSSEStream(context.Background(), nil, ch)

	chunk := <-ch
	if !errors.Is(chunk.Err, ErrNoSSEReader) {
		t.Errorf("expected ErrNoSSEReader, got %v", chunk.Err)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation during streaming
// ---------------------------------------------------------------------------

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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	first := <-ch
	if first.Err != nil {
		t.Fatalf("first chunk error: %v", first.Err)
	}
	if first.Content != "first" {
		t.Errorf("first content = %q, want %q", first.Content, "first")
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

// ---------------------------------------------------------------------------
// Adapter error recovery: BuildRequest fails
// ---------------------------------------------------------------------------

func TestAdapter_Execute_DialectBuildRequestError(t *testing.T) {
	buildFail := errors.New("transient build failure")
	d := &mockDialect{buildErr: buildFail}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = a.Execute(context.Background(), CompletionRequest{
		Messages: []Message{User("test")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error = %q, want to contain 'build request'", err.Error())
	}
}

func TestAdapter_Stream_DialectBuildRequestError(t *testing.T) {
	d := &mockDialect{buildErr: errors.New("build fail")}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = a.Stream(context.Background(), CompletionRequest{})
	if err == nil {
		t.Fatal("expected error from Stream when BuildRequest fails")
	}
	if !strings.Contains(err.Error(), "build stream request") {
		t.Errorf("error = %q, want to contain 'build stream request'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Config & defaults edge cases
// ---------------------------------------------------------------------------

func TestConfig_ApplyDefaults_EmptyDialect(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()

	if cfg.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want 120s", cfg.Timeout)
	}
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty when dialect is empty", cfg.Name)
	}
}

func TestCompletionRequest_ZeroTemperature(t *testing.T) {
	zero := 0.0
	req := CompletionRequest{
		Messages:    []Message{User("test")},
		Temperature: &zero,
	}
	if req.Temperature == nil {
		t.Fatal("Temperature should not be nil when set to 0")
	}
	if *req.Temperature != 0.0 {
		t.Errorf("Temperature = %v, want 0.0", *req.Temperature)
	}
}

func TestCompletionRequest_MaxTokensZero(t *testing.T) {
	req := CompletionRequest{
		Messages:  []Message{User("test")},
		MaxTokens: 0,
	}
	if req.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0 (provider default)", req.MaxTokens)
	}
}

func TestAdapter_ApplyDefaults_RequestOverridesConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		if body["model"] != "override-model" {
			t.Errorf("model = %v, want override-model", body["model"])
		}
		json.NewEncoder(w).Encode(map[string]any{"content": "ok", "model": "override-model"})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "default-model"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = a.Execute(context.Background(), CompletionRequest{
		Model:    "override-model",
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestAdapter_ApplyDefaults_ZeroTempNotOverridden(t *testing.T) {
	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{
		BaseURL:     "http://localhost:1",
		Temperature: 0.0,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	req := CompletionRequest{
		Messages: []Message{User("test")},
	}
	a.applyDefaults(&req)

	if req.Temperature != nil {
		t.Errorf("Temperature = %v, want nil (zero config temp should not be applied)", req.Temperature)
	}
}

// ---------------------------------------------------------------------------
// Usage tracking
// ---------------------------------------------------------------------------

func TestUsage_ZeroValues(t *testing.T) {
	u := Usage{}
	if u.PromptTokens != 0 || u.CompletionTokens != 0 || u.TotalTokens != 0 {
		t.Errorf("zero Usage = %+v, want all zeros", u)
	}
}

func TestUsage_JSON_RoundTrip(t *testing.T) {
	u := Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var u2 Usage
	if err := json.Unmarshal(data, &u2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u2 != u {
		t.Errorf("round-trip = %+v, want %+v", u2, u)
	}
}

// ---------------------------------------------------------------------------
// Execute with empty messages
// ---------------------------------------------------------------------------

func TestAdapter_Execute_EmptyMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"content": "ok", "model": "m"})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	resp, err := a.Execute(context.Background(), CompletionRequest{Messages: []Message{}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Text() != "ok" {
		t.Errorf("Text() = %q, want %q", resp.Text(), "ok")
	}
}

// ---------------------------------------------------------------------------
// Close idempotency
// ---------------------------------------------------------------------------

func TestAdapter_Close_Idempotent(t *testing.T) {
	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := a.Close(context.Background()); err != nil {
			t.Errorf("Close() call %d: %v", i+1, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Very large message content in Execute
// ---------------------------------------------------------------------------

func TestAdapter_Execute_LargeMessageContent(t *testing.T) {
	largeMsg := strings.Repeat("A", 128*1024) // 128 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just verify the request arrives and respond
		json.NewEncoder(w).Encode(map[string]any{"content": "ok", "model": "m"})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "m"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	resp, err := a.Execute(context.Background(), CompletionRequest{
		Messages: []Message{User(largeMsg)},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Text() != "ok" {
		t.Errorf("Text() = %q, want %q", resp.Text(), "ok")
	}
}

// ---------------------------------------------------------------------------
// Dialect registration edge cases
// ---------------------------------------------------------------------------

func TestRegisterDialect_NilDialect(t *testing.T) {
	dialectsMu.Lock()
	original := dialects
	dialects = map[string]Dialect{}
	dialectsMu.Unlock()
	defer func() {
		dialectsMu.Lock()
		dialects = original
		dialectsMu.Unlock()
	}()

	RegisterDialect("nil-d", nil)
	got, err := GetDialect("nil-d")
	if err != nil {
		t.Fatalf("GetDialect: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil dialect, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error identity
// ---------------------------------------------------------------------------

func TestSentinelErrors(t *testing.T) {
	if ErrNoDialect.Error() != "llm: dialect is required" {
		t.Errorf("ErrNoDialect = %q", ErrNoDialect)
	}
	if ErrNoSSEReader.Error() != "llm: expected SSE stream but got no SSE reader" {
		t.Errorf("ErrNoSSEReader = %q", ErrNoSSEReader)
	}
	if ErrNoStreamBody.Error() != "llm: expected stream body but got nil" {
		t.Errorf("ErrNoStreamBody = %q", ErrNoStreamBody)
	}
}

// ---------------------------------------------------------------------------
// StreamChunk with Done=true and content
// ---------------------------------------------------------------------------

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
		Messages: []Message{User("test")},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	chunk := <-ch
	if chunk.Err != nil {
		t.Fatalf("chunk error: %v", chunk.Err)
	}
	if chunk.Content != "final" {
		t.Errorf("Content = %q, want %q", chunk.Content, "final")
	}
	if !chunk.Done {
		t.Error("expected Done=true")
	}
}

// ---------------------------------------------------------------------------
// CompletionResponse & StreamChunk JSON serialization
// ---------------------------------------------------------------------------

func TestCompletionResponse_JSON_RoundTrip(t *testing.T) {
	resp := CompletionResponse{
		Message: Assistant("hello world"),
		Model:   "gpt-4",
		Usage:   Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
	}
	if resp.Text() != "hello world" {
		t.Errorf("Text() = %q, want %q", resp.Text(), "hello world")
	}
	if resp.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-4")
	}
}

func TestStreamChunk_JSON_Serialization(t *testing.T) {
	chunk := StreamChunk{Content: "hello", Done: false}
	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var chunk2 StreamChunk
	if err := json.Unmarshal(data, &chunk2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if chunk2.Content != "hello" || chunk2.Done != false {
		t.Errorf("round-trip = %+v", chunk2)
	}
}

// ---------------------------------------------------------------------------
// Message types and constructors
// ---------------------------------------------------------------------------

func TestMessageConstructors(t *testing.T) {
	u := User("hello")
	if u.Role() != RoleUser {
		t.Errorf("User.Role() = %q, want %q", u.Role(), RoleUser)
	}
	if TextOf(u.Content) != "hello" {
		t.Errorf("User content = %q, want %q", TextOf(u.Content), "hello")
	}

	a := Assistant("response")
	if a.Role() != RoleAssistant {
		t.Errorf("Assistant.Role() = %q, want %q", a.Role(), RoleAssistant)
	}
	if a.Text() != "response" {
		t.Errorf("Assistant.Text() = %q, want %q", a.Text(), "response")
	}

	s := System("you are helpful")
	if s.Role() != RoleSystem {
		t.Errorf("System.Role() = %q, want %q", s.Role(), RoleSystem)
	}
	if s.Content != "you are helpful" {
		t.Errorf("System.Content = %q, want %q", s.Content, "you are helpful")
	}

	tr := ToolResultMsg("id-1", "result data", false)
	if tr.Role() != RoleTool {
		t.Errorf("ToolResult.Role() = %q, want %q", tr.Role(), RoleTool)
	}
	if tr.ToolUseID != "id-1" {
		t.Errorf("ToolResult.ToolUseID = %q, want %q", tr.ToolUseID, "id-1")
	}
}

func TestMarshalMessage_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		role string
	}{
		{"user", User("hi"), "user"},
		{"assistant", Assistant("hello"), "assistant"},
		{"system", System("prompt"), "system"},
		{"tool_result", ToolResultMsg("id", "data", false), "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := MarshalMessage(tt.msg)
			if err != nil {
				t.Fatalf("MarshalMessage: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if raw["role"] != tt.role {
				t.Errorf("role = %q, want %q", raw["role"], tt.role)
			}
		})
	}
}

func TestStopReason_Values(t *testing.T) {
	tests := []struct {
		sr   StopReason
		want string
	}{
		{StopEndTurn, "end_turn"},
		{StopToolUse, "tool_use"},
		{StopMaxTokens, "max_tokens"},
		{StopContentFilter, "content_filter"},
		{StopSequence, "stop_sequence"},
	}
	for _, tt := range tests {
		if string(tt.sr) != tt.want {
			t.Errorf("StopReason = %q, want %q", tt.sr, tt.want)
		}
	}
}

func TestContentBlocks(t *testing.T) {
	tb := TextBlock{Text: "hello"}
	if tb.BlockType() != "text" {
		t.Errorf("TextBlock.BlockType() = %q", tb.BlockType())
	}

	ib := ImageBlock{Source: "url", MimeType: "image/png"}
	if ib.BlockType() != "image" {
		t.Errorf("ImageBlock.BlockType() = %q", ib.BlockType())
	}

	tub := ToolUseBlock{ID: "1", Name: "test"}
	if tub.BlockType() != "tool_use" {
		t.Errorf("ToolUseBlock.BlockType() = %q", tub.BlockType())
	}

	trb := ToolResultBlock{ToolUseID: "1", Content: "ok"}
	if trb.BlockType() != "tool_result" {
		t.Errorf("ToolResultBlock.BlockType() = %q", trb.BlockType())
	}

	thb := ThinkingBlock{Text: "reasoning"}
	if thb.BlockType() != "thinking" {
		t.Errorf("ThinkingBlock.BlockType() = %q", thb.BlockType())
	}
}
