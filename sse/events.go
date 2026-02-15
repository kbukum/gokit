// Package sse provides Server-Sent Events (SSE) support for real-time streaming.
package sse

// Generic SSE event type constants (infrastructure only).
// Domain-specific event types should be defined in your application.
const (
	// EventTypeConnected is sent when a client successfully connects.
	EventTypeConnected = "connected"

	// EventTypeKeepAlive is used for keep-alive comments.
	EventTypeKeepAlive = "keepalive"

	// EventTypeMessage is a generic message event.
	EventTypeMessage = "message"

	// EventTypeError is sent when an error occurs.
	EventTypeError = "error"

	// EventTypeMetric is sent for metric/telemetry events.
	EventTypeMetric = "metric"
)
