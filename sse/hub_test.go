package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		if string(msg) != "test message" {
			t.Errorf("expected 'test message', got '%s'", string(msg))
		}
	default:
		t.Error("expected message in channel")
	}
}

func TestClient_Send_ChannelFull(t *testing.T) {
	client := NewClient("test:abc123")

	// Fill the channel (size is 256)
	for i := 0; i < 256; i++ {
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
		if string(msg) != "message for abc" {
			t.Errorf("expected 'message for abc', got '%s'", string(msg))
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
		if string(msg) != "message for executions" {
			t.Errorf("client1: expected 'message for executions', got '%s'", string(msg))
		}
	default:
		t.Error("client1 should have received message")
	}

	// client2 should receive
	select {
	case msg := <-client2.Events():
		if string(msg) != "message for executions" {
			t.Errorf("client2: expected 'message for executions', got '%s'", string(msg))
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
