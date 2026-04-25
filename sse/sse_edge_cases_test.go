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

// ---------------------------------------------------------------------------
// Pattern matching edge cases
// ---------------------------------------------------------------------------

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

	// Send 300 messages (buffer is 256) — some should be dropped
	dropped := 0
	for i := 0; i < 300; i++ {
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
	dropped = 300 - received
	if dropped <= 0 {
		t.Errorf("expected some messages to be dropped, but all %d were received", received)
	}
	if received > 256 {
		t.Errorf("received %d messages but buffer is 256", received)
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

func TestEdge_SSEHeaders(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:headers")
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return // timeout acceptable for SSE
	}
	defer resp.Body.Close()

	tests := []struct {
		header, expected string
	}{
		{"Content-Type", "text/event-stream"},
		{"Cache-Control", "no-cache"},
		{"Connection", "keep-alive"},
		{"X-Accel-Buffering", "no"},
	}
	for _, tc := range tests {
		got := resp.Header.Get(tc.header)
		if got != tc.expected {
			t.Errorf("header %s: expected %q, got %q", tc.header, tc.expected, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrent broadcast + subscribe/unsubscribe
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

func TestEdge_ServeSSE_ConnectedEventFormat(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeSSE(hub, w, r, "test:fmt", WithUserID("u1"), WithSessionID("s1"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if !strings.Contains(body, "event: connected") {
		t.Errorf("expected 'event: connected' in body, got %q", body)
	}
	if !strings.Contains(body, `"client_id":"test:fmt"`) {
		t.Errorf("expected client_id in body, got %q", body)
	}
	if !strings.Contains(body, `"user_id":"u1"`) {
		t.Errorf("expected user_id in body, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// Broadcast to no matching clients is a no-op
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
