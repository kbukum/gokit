// Package sse provides Server-Sent Events (SSE) support for real-time streaming.
package sse

// Broadcaster is an interface for broadcasting events to clients.
// This allows handlers to depend on an abstraction rather than a concrete Hub.
type Broadcaster interface {
	// BroadcastToPattern sends data to all clients matching the given pattern.
	// Pattern uses glob-style matching (e.g., "resource:*" or "resource:abc123").
	BroadcastToPattern(pattern string, data []byte)
}
