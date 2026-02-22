package sse

import (
	"path/filepath"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// Client represents a connected SSE client.
type Client struct {
	id       string            // Unique client ID
	metadata map[string]string // Optional metadata (userID, sessionID, etc.)
	events   chan []byte       // Channel for sending events to client
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
		events:   make(chan []byte, 256), // Buffered channel
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

// Events returns the channel for receiving events.
func (c *Client) Events() <-chan []byte {
	return c.events
}

// Send sends data to the client's event channel.
// Returns false if the channel is full (client is slow).
func (c *Client) Send(data []byte) bool {
	select {
	case c.events <- data:
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
	stopped    bool              // Whether the hub has been stopped
	mu         sync.RWMutex       // Protects clients map for reads during matching
}

// Message represents a message to broadcast.
type Message struct {
	Pattern string // Glob pattern for matching clients
	Data    []byte // Event data to send
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
			h.broadcastWithPattern(msg.Pattern, msg.Data)
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
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// BroadcastToPattern sends data to all clients matching the pattern.
// Pattern uses glob-style matching (e.g., "execution:*" or "execution:abc123").
func (h *Hub) BroadcastToPattern(pattern string, data []byte) {
	h.broadcast <- &Message{
		Pattern: pattern,
		Data:    data,
	}
}

// broadcastWithPattern sends data to matching clients.
// This is called from the hub's main goroutine.
func (h *Hub) broadcastWithPattern(pattern string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	matchCount := 0
	for clientID, client := range h.clients {
		matched, err := filepath.Match(pattern, clientID)
		if err != nil {
			logger.Error("[SSE_HUB] Pattern match error", map[string]interface{}{
				"pattern": pattern,
				"error":   err.Error(),
			})
			continue
		}
		if matched {
			if client.Send(data) {
				matchCount++
			}
		}
	}

	if matchCount > 0 {
		logger.Debug("[SSE_HUB] Broadcast sent",
			map[string]interface{}{
				"pattern":     pattern,
				"match_count": matchCount,
				"data_size":   len(data),
			},
		)
	} else {
		logger.Debug("[SSE_HUB] No clients matched pattern",
			map[string]interface{}{
				"pattern":       pattern,
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
