// Package sse reads Server-Sent Events from an HTTP response stream.
//
// [NewReader] wraps a byte stream and yields decoded [Event] values (data,
// event type, and id) following the SSE line protocol, so httpclient consumers
// can process streaming endpoints without reimplementing the framing.
package sse
