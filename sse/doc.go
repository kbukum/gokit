// Package sse provides Server-Sent Events (SSE) infrastructure for
// real-time event delivery in gokit applications.
//
// It includes client connection management, event broadcasting to
// multiple subscribers, and a hub for managing event channels.
//
// # Architecture
//
//   - Hub: Central event router managing client subscriptions
//   - Broadcaster: Sends events to all connected clients
//   - Handler: HTTP handler for SSE endpoint
//
// # Usage
//
//	hub := sse.NewHub()
//	go hub.Run()
//	handler := sse.NewHandler(hub)
//	router.GET("/events", handler.ServeHTTP)
package sse
