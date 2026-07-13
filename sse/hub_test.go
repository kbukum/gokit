package sse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_NewClient(t *testing.T) {
	client := NewClient("test:abc123")

	if client.ID() != "test:abc123" {
		t.Errorf("expected ID 'test:abc123', got '%s'", client.ID())
	}

	if client.Events() == nil {
		t.Error("expected events channel to be set")
	}
}

func TestClient_Send_Success(t *testing.T) {
	client := NewClient("test:abc123")

	// Send a message
	ok := client.Send([]byte("test message"))
	if !ok {
		t.Error("expected send to succeed")
	}

	// Verify message is in channel
	select {
	case msg := <-client.Events():
		if string(msg.Data) != "test message" {
			t.Errorf("expected 'test message', got '%s'", string(msg.Data))
		}
	default:
		t.Error("expected message in channel")
	}
}

func TestClient_Send_ChannelFull(t *testing.T) {
	client := NewClient("test:abc123")

	// Fill the channel to its bounded backpressure limit.
	for i := 0; i < DefaultClientBufferSize; i++ {
		client.Send([]byte("msg"))
	}

	// Next send should fail (channel full)
	ok := client.Send([]byte("overflow"))
	if ok {
		t.Error("expected send to fail when channel is full")
	}
}

func TestClient_Close(t *testing.T) {
	client := NewClient("test:abc123")
	client.Close()

	// Channel should be closed
	_, open := <-client.Events()
	if open {
		t.Error("expected channel to be closed")
	}
}

func TestHub_NewHub(t *testing.T) {
	hub := NewHub()

	if hub == nil {
		t.Fatal("expected hub to be created")
	}

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.GetClientCount())
	}
}

func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := NewClient("test:abc123")

	// Register client
	hub.Register(client)
	time.Sleep(10 * time.Millisecond) // Wait for registration

	if hub.GetClientCount() != 1 {
		t.Errorf("expected 1 client after register, got %d", hub.GetClientCount())
	}

	// Unregister client
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond) // Wait for unregistration

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.GetClientCount())
	}
}

func TestHub_GetClientIDs(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client1 := NewClient("test:abc")
	client2 := NewClient("test:xyz")

	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(10 * time.Millisecond)

	ids := hub.GetClientIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 client IDs, got %d", len(ids))
	}

	// Verify both IDs are present
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	if !idMap["test:abc"] {
		t.Error("expected 'test:abc' in client IDs")
	}
	if !idMap["test:xyz"] {
		t.Error("expected 'test:xyz' in client IDs")
	}
}

func TestHub_BroadcastToPattern_ExactMatch(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client1 := NewClient("test:abc123")
	client2 := NewClient("test:xyz789")

	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(10 * time.Millisecond)

	// Broadcast to exact match
	hub.BroadcastToPattern("test:abc123", []byte("message for abc"))
	time.Sleep(10 * time.Millisecond)

	// client1 should receive
	select {
	case msg := <-client1.Events():
		if string(msg.Data) != "message for abc" {
			t.Errorf("expected 'message for abc', got '%s'", string(msg.Data))
		}
	default:
		t.Error("client1 should have received message")
	}

	// client2 should NOT receive
	select {
	case <-client2.Events():
		t.Error("client2 should NOT have received message")
	default:
		// Expected
	}
}

func TestHub_BroadcastToPattern_Wildcard(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client1 := NewClient("test:abc")
	client2 := NewClient("test:xyz")
	client3 := NewClient("pipeline:abc")

	hub.Register(client1)
	hub.Register(client2)
	hub.Register(client3)
	time.Sleep(10 * time.Millisecond)

	// Broadcast to all execution clients
	hub.BroadcastToPattern("test:*", []byte("message for executions"))
	time.Sleep(10 * time.Millisecond)

	// client1 should receive
	select {
	case msg := <-client1.Events():
		if string(msg.Data) != "message for executions" {
			t.Errorf("client1: expected 'message for executions', got '%s'", string(msg.Data))
		}
	default:
		t.Error("client1 should have received message")
	}

	// client2 should receive
	select {
	case msg := <-client2.Events():
		if string(msg.Data) != "message for executions" {
			t.Errorf("client2: expected 'message for executions', got '%s'", string(msg.Data))
		}
	default:
		t.Error("client2 should have received message")
	}

	// client3 (pipeline) should NOT receive
	select {
	case <-client3.Events():
		t.Error("client3 should NOT have received execution message")
	default:
		// Expected
	}
}

func TestHub_ConcurrentOperations(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var wg sync.WaitGroup
	clients := make([]*Client, 10)

	// Register clients concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clients[idx] = NewClient("test:client-" + string(rune('a'+idx)))
			hub.Register(clients[idx])
		}(i)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	if hub.GetClientCount() != 10 {
		t.Errorf("expected 10 clients, got %d", hub.GetClientCount())
	}

	// Broadcast concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.BroadcastToPattern("test:*", []byte("concurrent message"))
		}()
	}
	wg.Wait()

	// Unregister concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.Unregister(clients[idx])
		}(i)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.GetClientCount())
	}
}

func TestMessage_Fields(t *testing.T) {
	msg := &Message{
		Pattern: "test:*",
		Data:    []byte("test data"),
	}

	if msg.Pattern != "test:*" {
		t.Errorf("expected pattern 'test:*', got '%s'", msg.Pattern)
	}

	if string(msg.Data) != "test data" {
		t.Errorf("expected data 'test data', got '%s'", string(msg.Data))
	}
}

func TestClient_WithMetadata(t *testing.T) {
	client := NewClient("test:abc",
		WithMetadata("custom-key", "custom-value"),
	)

	if client.GetMetadata("custom-key") != "custom-value" {
		t.Errorf("expected metadata 'custom-value', got '%s'", client.GetMetadata("custom-key"))
	}
}

func TestClient_WithUserID(t *testing.T) {
	client := NewClient("test:abc",
		WithUserID("user-123"),
	)

	if client.UserID() != "user-123" {
		t.Errorf("expected UserID 'user-123', got '%s'", client.UserID())
	}
	if client.GetMetadata("user_id") != "user-123" {
		t.Errorf("expected metadata user_id 'user-123', got '%s'", client.GetMetadata("user_id"))
	}
}

func TestClient_WithSessionID(t *testing.T) {
	client := NewClient("test:abc",
		WithSessionID("session-456"),
	)

	if client.SessionID() != "session-456" {
		t.Errorf("expected SessionID 'session-456', got '%s'", client.SessionID())
	}
}

func TestClient_MultipleOptions(t *testing.T) {
	client := NewClient("test:abc",
		WithUserID("user-1"),
		WithSessionID("sess-2"),
		WithMetadata("env", "prod"),
	)

	if client.UserID() != "user-1" {
		t.Errorf("expected UserID 'user-1', got '%s'", client.UserID())
	}
	if client.SessionID() != "sess-2" {
		t.Errorf("expected SessionID 'sess-2', got '%s'", client.SessionID())
	}
	if client.GetMetadata("env") != "prod" {
		t.Errorf("expected env 'prod', got '%s'", client.GetMetadata("env"))
	}
}

func TestClient_Metadata(t *testing.T) {
	client := NewClient("test:abc",
		WithMetadata("k1", "v1"),
		WithMetadata("k2", "v2"),
	)

	meta := client.Metadata()
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if len(meta) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(meta))
	}
}

func TestHub_GetClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := NewClient("test:abc123")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	got := hub.GetClient("test:abc123")
	if got == nil {
		t.Error("expected to find registered client")
	}
	if got.ID() != "test:abc123" {
		t.Errorf("expected ID 'test:abc123', got '%s'", got.ID())
	}

	missing := hub.GetClient("nonexistent")
	if missing != nil {
		t.Error("expected nil for unregistered client")
	}
}

func TestHub_Stop(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := NewClient("test:abc")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// Double stop should be safe
	hub.Stop()
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent("/events")

	if comp.Name() != "sse" {
		t.Errorf("expected name 'sse', got %q", comp.Name())
	}

	// Start
	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Health
	health := comp.Health(ctx)
	if health.Name != "sse" {
		t.Errorf("expected health name 'sse', got %q", health.Name)
	}
	if health.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", health.Status)
	}
	if !strings.Contains(health.Message, "0 clients") {
		t.Errorf("expected '0 clients' in message, got %q", health.Message)
	}

	// Hub should be accessible
	if comp.Hub() == nil {
		t.Error("expected non-nil Hub")
	}

	// Stop
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestComponent_Describe(t *testing.T) {
	comp := NewComponent("/api/events")

	desc := comp.Describe()
	if desc.Name != "SSE Hub" {
		t.Errorf("expected name 'SSE Hub', got %q", desc.Name)
	}
	if desc.Type != "sse" {
		t.Errorf("expected type 'sse', got %q", desc.Type)
	}
	if !strings.Contains(desc.Details, "/api/events") {
		t.Errorf("expected path in details, got %q", desc.Details)
	}
}

func TestComponent_WithClients(t *testing.T) {
	comp := NewComponent("/events")
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	// Register a client through the hub
	client := NewClient("test:client-1")
	comp.Hub().Register(client)
	time.Sleep(10 * time.Millisecond)

	health := comp.Health(ctx)
	if !strings.Contains(health.Message, "1 clients") {
		t.Errorf("expected '1 clients' in message, got %q", health.Message)
	}
}

func TestServeSSE(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// Create a test HTTP server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:client-1", WithUserID("user-1"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect as SSE client
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Context timeout is expected - we just want to verify the connection was established
		return
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %q", resp.Header.Get("Cache-Control"))
	}
}

func TestServeSSE_WithBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:client-1")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect as SSE client in background
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return // timeout is ok for SSE
	}
	defer resp.Body.Close()

	// Read some data (connected event)
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	data := string(buf[:n])

	if !strings.Contains(data, "connected") {
		t.Errorf("expected connected event, got %q", data)
	}
}

func TestEventTypeConstants(t *testing.T) {
	if EventTypeConnected != "connected" {
		t.Errorf("expected 'connected', got %q", EventTypeConnected)
	}
	if EventTypeKeepAlive != "keepalive" {
		t.Errorf("expected 'keepalive', got %q", EventTypeKeepAlive)
	}
	if EventTypeMessage != "message" {
		t.Errorf("expected 'message', got %q", EventTypeMessage)
	}
	if EventTypeError != "error" {
		t.Errorf("expected 'error', got %q", EventTypeError)
	}
	if EventTypeMetric != "metric" {
		t.Errorf("expected 'metric', got %q", EventTypeMetric)
	}
}

// TestHub_Broadcast_NamedEvent verifies that Broadcast(event, data) delivers
// to all connected clients with the SSE `event:` line set, and is glob-matched
// against "*". This is the path the frontend EventSource named-event
// listeners (`source.addEventListener(name, ...)`) actually fire on.
func TestHub_Broadcast_NamedEvent(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c1 := NewClient("u-1")
	c2 := NewClient("u-2")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(10 * time.Millisecond)

	hub.Broadcast("notifications.new", []byte(`{"id":"x"}`))
	time.Sleep(10 * time.Millisecond)

	for _, c := range []*Client{c1, c2} {
		select {
		case f := <-c.Events():
			if f.Event != "notifications.new" {
				t.Errorf("client %s: expected event 'notifications.new', got %q", c.ID(), f.Event)
			}
			if string(f.Data) != `{"id":"x"}` {
				t.Errorf("client %s: unexpected data %q", c.ID(), string(f.Data))
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("client %s: timed out waiting for frame", c.ID())
		}
	}
}

// TestServeSSE_WritesEventLine verifies the wire format includes the
// `event: <name>` line so browser EventSource named-event listeners fire.
func TestServeSSE_WritesEventLine(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "wire-test")
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer resp.Body.Close()

	// Drain initial connected event.
	buf := make([]byte, 4096)
	if _, readErr := resp.Body.Read(buf); readErr != nil {
		t.Fatalf("read connect: %v", readErr)
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		hub.Broadcast("test.event", []byte(`{"k":1}`))
	}()

	// Read the broadcast frame.
	n, err := resp.Body.Read(buf)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	frame := string(buf[:n])
	if !strings.Contains(frame, "event: test.event\n") {
		t.Errorf("missing `event:` line in wire frame: %q", frame)
	}
	if !strings.Contains(frame, `data: {"k":1}`) {
		t.Errorf("missing data line: %q", frame)
	}
}

func TestEdge_PatternMatching_InvalidGlob(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := NewClient("test:abc")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// filepath.Match returns an error for malformed patterns like "[abc"
	// The hub should log the error and not panic.
	hub.BroadcastToPattern("[invalid", []byte("should not crash"))
	time.Sleep(20 * time.Millisecond)

	// Client should NOT receive the message since the pattern is invalid
	select {
	case <-client.Events():
		t.Error("client should not receive message for invalid pattern")
	default:
		// Expected: no message delivered
	}
}

func TestEdge_PatternMatching_SpecialCharacters(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// Client IDs with special characters
	c1 := NewClient("user:john.doe@example.com")
	c2 := NewClient("user:jane-doe_123")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(10 * time.Millisecond)

	// Exact match with special characters
	hub.BroadcastToPattern("user:john.doe@example.com", []byte("hit"))
	time.Sleep(10 * time.Millisecond)

	select {
	case msg := <-c1.Events():
		if string(msg.Data) != "hit" {
			t.Errorf("expected 'hit', got %q", string(msg.Data))
		}
	default:
		t.Error("c1 should have received the message")
	}
	select {
	case <-c2.Events():
		t.Error("c2 should not receive message for a different exact match")
	default:
	}
}

func TestEdge_PatternMatching_EmptyPattern(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("test:abc")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	// Empty pattern matches only empty string (filepath.Match("","test:abc") == false)
	hub.BroadcastToPattern("", []byte("nothing"))
	time.Sleep(10 * time.Millisecond)

	select {
	case <-c.Events():
		t.Error("empty pattern should not match non-empty client ID")
	default:
	}
}

func TestEdge_PatternMatching_EmptyClientID(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	// Empty pattern matches empty client ID
	hub.BroadcastToPattern("", []byte("hit"))
	time.Sleep(10 * time.Millisecond)

	select {
	case msg := <-c.Events():
		if string(msg.Data) != "hit" {
			t.Errorf("expected 'hit', got %q", string(msg.Data))
		}
	default:
		t.Error("empty pattern should match empty client ID")
	}
}

func TestEdge_PatternMatching_QuestionMarkWildcard(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c1 := NewClient("a1")
	c2 := NewClient("a2")
	c3 := NewClient("ab1")
	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)
	time.Sleep(10 * time.Millisecond)

	// ? matches exactly one character
	hub.BroadcastToPattern("a?", []byte("hit"))
	time.Sleep(10 * time.Millisecond)

	for _, c := range []*Client{c1, c2} {
		select {
		case msg := <-c.Events():
			if string(msg.Data) != "hit" {
				t.Errorf("client %s: expected 'hit', got %q", c.ID(), string(msg.Data))
			}
		default:
			t.Errorf("client %s should have matched 'a?'", c.ID())
		}
	}

	select {
	case <-c3.Events():
		t.Error("'ab1' should NOT match 'a?' (too long)")
	default:
	}
}

func TestEdge_PatternMatching_CharacterClass(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c1 := NewClient("a")
	c2 := NewClient("b")
	c3 := NewClient("c")
	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastToPattern("[ab]", []byte("hit"))
	time.Sleep(10 * time.Millisecond)

	for _, c := range []*Client{c1, c2} {
		select {
		case msg := <-c.Events():
			if string(msg.Data) != "hit" {
				t.Errorf("client %s: expected 'hit', got %q", c.ID(), string(msg.Data))
			}
		default:
			t.Errorf("client %s should match '[ab]'", c.ID())
		}
	}

	select {
	case <-c3.Events():
		t.Error("client 'c' should NOT match '[ab]'")
	default:
	}
}

// ---------------------------------------------------------------------------
// Large payload
// ---------------------------------------------------------------------------

func TestEdge_LargePayloadBroadcast(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("test:large")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	// 1 MB payload
	largeData := make([]byte, 1<<20)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}

	hub.BroadcastToPattern("test:large", largeData)
	time.Sleep(50 * time.Millisecond)

	select {
	case msg := <-c.Events():
		if len(msg.Data) != 1<<20 {
			t.Errorf("expected 1MB payload, got %d bytes", len(msg.Data))
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for large payload")
	}
}

// ---------------------------------------------------------------------------
// Slow client handling (buffer overflow)
// ---------------------------------------------------------------------------

func TestEdge_SlowClient_BufferOverflow(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	slow := NewClient("test:slow")
	hub.Register(slow)
	time.Sleep(10 * time.Millisecond)

	// Send more than the bounded client queue size — some should be dropped.
	for i := 0; i < DefaultClientBufferSize+44; i++ {
		hub.BroadcastToPattern("test:slow", []byte(fmt.Sprintf("msg-%d", i)))
	}
	time.Sleep(50 * time.Millisecond)

	received := 0
	for {
		select {
		case <-slow.Events():
			received++
		default:
			goto done
		}
	}
done:
	dropped := (DefaultClientBufferSize + 44) - received
	if dropped <= 0 {
		t.Errorf("expected some messages to be dropped, but all %d were received", received)
	}
	if received > DefaultClientBufferSize {
		t.Errorf("received %d messages but buffer is %d", received, DefaultClientBufferSize)
	}
}

// ---------------------------------------------------------------------------
// Memory cleanup after Hub.Stop()
// ---------------------------------------------------------------------------

func TestEdge_MemoryCleanupAfterStop(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()

	for i := 0; i < 50; i++ {
		c := NewClient(fmt.Sprintf("test:client-%d", i))
		hub.Register(c)
	}
	time.Sleep(20 * time.Millisecond)

	if hub.GetClientCount() != 50 {
		t.Fatalf("expected 50 clients, got %d", hub.GetClientCount())
	}

	hub.Stop()
	time.Sleep(20 * time.Millisecond)

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after stop, got %d", hub.GetClientCount())
	}
}

// ---------------------------------------------------------------------------
// SSE protocol header validation
// ---------------------------------------------------------------------------

func TestEdge_ConcurrentBroadcastAndSubscription(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	var wg sync.WaitGroup

	// Continuously broadcast in one goroutine
	wg.Add(1)
	stop := make(chan struct{})
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				hub.BroadcastToPattern("test:*", []byte(fmt.Sprintf("msg-%d", i)))
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Rapidly register and unregister clients in parallel
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := NewClient(fmt.Sprintf("test:churn-%d", idx))
			hub.Register(c)
			time.Sleep(5 * time.Millisecond)
			hub.Unregister(c)
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	// Hub should still be operational — register a new client
	c := NewClient("test:after-churn")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastToPattern("test:after-churn", []byte("still alive"))
	time.Sleep(10 * time.Millisecond)

	select {
	case msg := <-c.Events():
		if string(msg.Data) != "still alive" {
			t.Errorf("expected 'still alive', got %q", string(msg.Data))
		}
	default:
		t.Error("hub should still be operational after churn")
	}
}

// ---------------------------------------------------------------------------
// Client metadata preservation
// ---------------------------------------------------------------------------

func TestEdge_ClientMetadataPreservation(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("test:meta",
		WithUserID("user-42"),
		WithSessionID("sess-99"),
		WithMetadata("role", "admin"),
		WithMetadata("lang", "en"),
	)
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	got := hub.GetClient("test:meta")
	if got == nil {
		t.Fatal("expected to find registered client")
	}

	// Verify all metadata is preserved
	if got.UserID() != "user-42" {
		t.Errorf("UserID: expected 'user-42', got %q", got.UserID())
	}
	if got.SessionID() != "sess-99" {
		t.Errorf("SessionID: expected 'sess-99', got %q", got.SessionID())
	}
	if got.GetMetadata("role") != "admin" {
		t.Errorf("role: expected 'admin', got %q", got.GetMetadata("role"))
	}
	if got.GetMetadata("lang") != "en" {
		t.Errorf("lang: expected 'en', got %q", got.GetMetadata("lang"))
	}
}

// ---------------------------------------------------------------------------
// Hub stats accuracy under concurrent modifications
// ---------------------------------------------------------------------------

func TestEdge_HubStatsAccuracyUnderConcurrency(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	const n = 50
	clients := make([]*Client, n)
	for i := 0; i < n; i++ {
		clients[i] = NewClient(fmt.Sprintf("test:stat-%d", i))
		hub.Register(clients[i])
	}
	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != n {
		t.Fatalf("expected %d clients, got %d", n, hub.GetClientCount())
	}

	ids := hub.GetClientIDs()
	if len(ids) != n {
		t.Fatalf("expected %d IDs, got %d", n, len(ids))
	}

	// Unregister half concurrently
	var wg sync.WaitGroup
	for i := 0; i < n/2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.Unregister(clients[idx])
		}(i)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	remaining := hub.GetClientCount()
	if remaining != n/2 {
		t.Errorf("expected %d clients remaining, got %d", n/2, remaining)
	}
}

// ---------------------------------------------------------------------------
// Register after Stop is safe
// ---------------------------------------------------------------------------

func TestEdge_RegisterAfterStop(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()

	hub.Stop()
	time.Sleep(10 * time.Millisecond)

	// Register/Unregister after Stop should not block or panic
	c := NewClient("test:late")
	hub.Register(c)
	hub.Unregister(c)
}

// ---------------------------------------------------------------------------
// ServeSSE connected event has expected format
// ---------------------------------------------------------------------------

func TestEdge_BroadcastNoMatch(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("test:abc")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastToPattern("other:*", []byte("miss"))
	time.Sleep(10 * time.Millisecond)

	select {
	case <-c.Events():
		t.Error("client should not receive message for non-matching pattern")
	default:
	}
}

// ---------------------------------------------------------------------------
// Multiple concurrent broadcasts with ordering per client
// ---------------------------------------------------------------------------

func TestEdge_ConcurrentBroadcastOrdering(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c := NewClient("test:order")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	const n = 100
	for i := 0; i < n; i++ {
		hub.BroadcastToPattern("test:order", []byte(fmt.Sprintf("%d", i)))
	}
	time.Sleep(50 * time.Millisecond)

	// Since BroadcastToPattern goes through the hub's single broadcast channel,
	// messages are processed sequentially — order should be preserved.
	for i := 0; i < n; i++ {
		select {
		case msg := <-c.Events():
			expected := fmt.Sprintf("%d", i)
			if string(msg.Data) != expected {
				t.Fatalf("at index %d: expected %q, got %q", i, expected, string(msg.Data))
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for message %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Wildcard * does not match across path separators in filepath.Match
// ---------------------------------------------------------------------------

func TestEdge_PatternMatching_WildcardDoesNotMatchSlash(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// filepath.Match("a:*", "a:b/c") returns false because * doesn't match /
	c := NewClient("a:b/c")
	hub.Register(c)
	time.Sleep(10 * time.Millisecond)

	hub.BroadcastToPattern("a:*", []byte("nope"))
	time.Sleep(10 * time.Millisecond)

	select {
	case <-c.Events():
		t.Error("* should not match / in filepath.Match")
	default:
	}
}

// ---------------------------------------------------------------------------
// Hub handles many simultaneous broadcasts
// ---------------------------------------------------------------------------

func TestEdge_ManySimultaneousBroadcasts(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	const numClients = 10
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = NewClient(fmt.Sprintf("test:multi-%d", i))
		hub.Register(clients[i])
	}
	time.Sleep(20 * time.Millisecond)

	const numBroadcasts = 50
	var wg sync.WaitGroup
	var sent atomic.Int64

	for i := 0; i < numBroadcasts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hub.BroadcastToPattern("test:*", []byte(fmt.Sprintf("broadcast-%d", idx)))
			sent.Add(1)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if sent.Load() != numBroadcasts {
		t.Errorf("expected %d broadcasts sent, got %d", numBroadcasts, sent.Load())
	}

	// Each client should have received messages (may not be all due to buffer)
	for _, c := range clients {
		count := 0
		for {
			select {
			case <-c.Events():
				count++
			default:
				goto nextClient
			}
		}
	nextClient:
		if count == 0 {
			t.Errorf("client %s received 0 messages", c.ID())
		}
	}
}
