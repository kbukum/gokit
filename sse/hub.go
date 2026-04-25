package sse

import (
	"path/filepath"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// Frame represents a single SSE event payload — an optional named event
// type plus the raw data bytes. When Event is non-empty, ServeSSE writes an
// `event: <name>` line so browser EventSource named-event listeners
// (`source.addEventListener("X", ...)`) fire. When empty, the frame is sent
// as a generic `message` event.
type Frame struct {
	Event string
	Data  []byte
}

// Client represents a connected SSE client.
type Client struct {
	id       string            // Unique client ID
	metadata map[string]string // Optional metadata (userID, sessionID, etc.)
	events   chan Frame        // Channel for sending events to client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithMetadata adds a metadata key-value pair to the client.
func WithMetadata(key, value string) ClientOption {
	return func(c *Client) {
		if c.metadata == nil {
			c.metadata = make(map[string]string)
		}
		c.metadata[key] = value
	}
}

// WithUserID sets the user ID metadata.
func WithUserID(userID string) ClientOption {
	return WithMetadata("user_id", userID)
}

// WithSessionID sets the session ID metadata.
func WithSessionID(sessionID string) ClientOption {
	return WithMetadata("session_id", sessionID)
}

// NewClient creates a new SSE client with optional metadata.
func NewClient(id string, opts ...ClientOption) *Client {
	c := &Client{
		id:       id,
		metadata: make(map[string]string),
		events:   make(chan Frame, 256), // Buffered channel
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ID returns the client's unique identifier.
func (c *Client) ID() string {
	return c.id
}

// Metadata returns all client metadata.
func (c *Client) Metadata() map[string]string {
	return c.metadata
}

// GetMetadata returns a specific metadata value.
func (c *Client) GetMetadata(key string) string {
	return c.metadata[key]
}

// UserID returns the client's user ID (convenience method).
func (c *Client) UserID() string {
	return c.metadata["user_id"]
}

// SessionID returns the client's session ID (convenience method).
func (c *Client) SessionID() string {
	return c.metadata["session_id"]
}

// Events returns the channel for receiving event frames.
func (c *Client) Events() <-chan Frame {
	return c.events
}

// Send sends raw data to the client as a generic SSE `message` event
// (no named event line). Returns false if the channel is full.
func (c *Client) Send(data []byte) bool {
	return c.SendFrame(Frame{Data: data})
}

// SendFrame sends a typed SSE frame. When frame.Event is non-empty, the
// browser's named-event listener for that event fires; otherwise the
// default `message` listener fires. Returns false if the channel is full.
func (c *Client) SendFrame(frame Frame) bool {
	select {
	case c.events <- frame:
		return true
	default:
		// Channel full, client is too slow
		logger.Warn("[SSE] Client channel full, dropping message", map[string]interface{}{
			"client_id": c.id,
		})
		return false
	}
}

// Close closes the client's event channel.
func (c *Client) Close() {
	close(c.events)
}

// Hub manages SSE client connections and message broadcasting.
type Hub struct {
	clients    map[string]*Client // client ID -> Client
	register   chan *Client       // Channel for registering clients
	unregister chan *Client       // Channel for unregistering clients
	broadcast  chan *Message      // Channel for broadcasting messages
	done       chan struct{}      // Signals the hub to stop
	stopped    bool               // Whether the hub has been stopped
	mu         sync.RWMutex       // Protects clients map for reads during matching
}

// Message represents a message to broadcast.
//
// Pattern selects which clients receive the message — glob-matched against
// each client's ID (e.g. "execution:*" or "execution:abc123"). Use "*" to
// reach every connected client.
//
// Event, when non-empty, becomes the SSE `event:` line so browser
// EventSource named-event listeners (`source.addEventListener("X", ...)`)
// fire. Leave empty for a generic `message` event.
type Message struct {
	Pattern string
	Event   string
	Data    []byte
}

// NewHub creates a new SSE hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		done:       make(chan struct{}),
	}
}

// Run starts the hub's main event loop.
// It blocks until Stop is called or the context is canceled.
// This should be run in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			h.closeAllClients()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.id] = client
			h.mu.Unlock()
			logger.Debug("[SSE_HUB] Client registered", map[string]interface{}{
				"client_id":     client.id,
				"total_clients": len(h.clients),
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.id]; ok {
				delete(h.clients, client.id)
				client.Close()
			}
			h.mu.Unlock()
			logger.Debug("[SSE_HUB] Client unregistered", map[string]interface{}{
				"client_id":     client.id,
				"total_clients": len(h.clients),
			})

		case msg := <-h.broadcast:
			h.dispatch(msg)
		}
	}
}

// Stop signals the hub to shut down. It closes all client connections
// and causes Run to return. Safe to call multiple times.
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		close(h.done)
	}
}

// closeAllClients disconnects all clients during shutdown.
func (h *Hub) closeAllClients() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, client := range h.clients {
		client.Close()
		delete(h.clients, id)
	}
	logger.Debug("[SSE_HUB] All clients closed during shutdown")
}

// Register adds a client to the hub.
// Returns immediately if the hub has been stopped.
func (h *Hub) Register(client *Client) {
	select {
	case h.register <- client:
	case <-h.done:
	}
}

// Unregister removes a client from the hub.
// Returns immediately if the hub has been stopped.
func (h *Hub) Unregister(client *Client) {
	select {
	case h.unregister <- client:
	case <-h.done:
	}
}

// Broadcast sends a typed event to ALL connected clients. The event name
// becomes the SSE `event:` line so browser EventSource named-event
// listeners (`source.addEventListener(event, ...)`) fire.
//
// This is the primary API for global notifications (e.g. "notifications.new",
// "catalog.pull.completed"). For per-resource fan-out where only some
// subscribers should receive the message, use BroadcastToPattern with a
// caller-defined client ID convention.
func (h *Hub) Broadcast(event string, data []byte) {
	h.broadcast <- &Message{
		Pattern: "*",
		Event:   event,
		Data:    data,
	}
}

// BroadcastToPattern sends data to all clients whose ID matches pattern.
// Pattern uses glob-style matching (e.g., "execution:*" or "execution:abc123").
// Use Broadcast for global, event-typed notifications.
func (h *Hub) BroadcastToPattern(pattern string, data []byte) {
	h.broadcast <- &Message{
		Pattern: pattern,
		Data:    data,
	}
}

// BroadcastFrame sends a typed frame to all clients whose ID matches
// pattern. Use Broadcast for the common "deliver to everyone" case.
func (h *Hub) BroadcastFrame(pattern string, frame Frame) {
	h.broadcast <- &Message{
		Pattern: pattern,
		Event:   frame.Event,
		Data:    frame.Data,
	}
}

// dispatch routes a Message to matching clients. Called from the hub's
// main goroutine.
func (h *Hub) dispatch(msg *Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	frame := Frame{Event: msg.Event, Data: msg.Data}
	matchCount := 0
	for clientID, client := range h.clients {
		matched, err := filepath.Match(msg.Pattern, clientID)
		if err != nil {
			logger.Error("[SSE_HUB] Pattern match error", map[string]interface{}{
				"pattern": msg.Pattern,
				"error":   err.Error(),
			})
			continue
		}
		if matched {
			if client.SendFrame(frame) {
				matchCount++
			}
		}
	}

	if matchCount > 0 {
		logger.Debug("[SSE_HUB] Broadcast sent",
			map[string]interface{}{
				"pattern":     msg.Pattern,
				"event":       msg.Event,
				"match_count": matchCount,
				"data_size":   len(msg.Data),
			},
		)
	} else {
		logger.Debug("[SSE_HUB] No clients matched pattern",
			map[string]interface{}{
				"pattern":       msg.Pattern,
				"event":         msg.Event,
				"total_clients": len(h.clients),
			},
		)
	}
}

// GetClientCount returns the number of connected clients.
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetClientIDs returns a list of all connected client IDs.
func (h *Hub) GetClientIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}

// GetClient returns a client by ID, or nil if not found.
func (h *Hub) GetClient(id string) *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.clients[id]
}

// Ensure Hub implements Broadcaster.
var _ Broadcaster = (*Hub)(nil)
