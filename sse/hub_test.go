package sse

import (
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
