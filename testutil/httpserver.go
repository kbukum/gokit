package testutil

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
)

// FakeHTTPServer is an in-process HTTP server for transport tests. It binds an
// ephemeral loopback port, serves a single programmed [FakeResponse], and
// records the first request for later inspection — no network dependency and no
// hand-rolled listener per test.
//
// The server is bounded to a single programmed request: the first request
// receives the programmed response, and any request beyond that budget is
// answered deterministically with the exhausted status (409 Conflict by
// default, override with [WithExhaustedStatus]) rather than hanging or
// re-serving. Only the first request is captured. Request bodies are bounded to
// [DefaultMaxBodyBytes] (override with [WithMaxBodyBytes]); a larger body is
// answered with 413 Request Entity Too Large rather than buffered in full. The
// server is safe for concurrent use and is drained and shut down by
// [FakeHTTPServer.Close].
type FakeHTTPServer struct {
	server          *httptest.Server
	response        *FakeResponse
	exhaustedStatus int
	maxBodyBytes    int64

	mu       sync.Mutex
	captured *CapturedRequest
	count    int
	writeErr error
}

// DefaultMaxBodyBytes bounds the request body a [FakeHTTPServer] reads unless
// overridden with [WithMaxBodyBytes]. It keeps a misbehaving or hostile client
// from growing the harness memory without bound.
const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

// ServerOption configures a [FakeHTTPServer] at construction time.
type ServerOption func(*FakeHTTPServer)

// WithExhaustedStatus overrides the status served for requests beyond the
// single-request budget. The default is [http.StatusConflict] (409).
func WithExhaustedStatus(status int) ServerOption {
	return func(s *FakeHTTPServer) { s.exhaustedStatus = status }
}

// WithMaxBodyBytes overrides the maximum request body size the server reads. A
// request whose body exceeds the limit is answered with 413 Request Entity Too
// Large rather than being buffered in full. The default is [DefaultMaxBodyBytes].
func WithMaxBodyBytes(limit int64) ServerOption {
	return func(s *FakeHTTPServer) { s.maxBodyBytes = limit }
}

// NewFakeHTTPServer starts a loopback server that answers the first request
// with response and returns immediately. Call [FakeHTTPServer.Close] to drain
// and shut it down; a *testing.T caller should defer it.
func NewFakeHTTPServer(response *FakeResponse, opts ...ServerOption) *FakeHTTPServer {
	s := &FakeHTTPServer{
		response:        response,
		exhaustedStatus: http.StatusConflict,
		maxBodyBytes:    DefaultMaxBodyBytes,
	}
	if s.response == nil {
		s.response = NewFakeResponse(http.StatusOK)
	}
	for _, opt := range opts {
		opt(s)
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// URL returns the base URL of the running server, e.g. "http://127.0.0.1:53812".
func (s *FakeHTTPServer) URL() string {
	return s.server.URL
}

// Addr returns the "host:port" the server is listening on.
func (s *FakeHTTPServer) Addr() string {
	return s.server.Listener.Addr().String()
}

// CapturedRequest returns the first request the server served, or nil if it has
// not received a request yet. Call it after driving the client under test.
func (s *FakeHTTPServer) CapturedRequest() *CapturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.captured
}

// RequestCount returns how many requests the server has received, including
// those beyond the single-request budget.
func (s *FakeHTTPServer) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// Close drains outstanding requests and shuts the server down, releasing the
// listener. It is safe to call once; subsequent connections are refused.
func (s *FakeHTTPServer) Close() {
	s.server.Close()
}

// handle records the request and serves the programmed response for the first
// request, or the exhausted status for any request beyond the budget.
func (s *FakeHTTPServer) handle(w http.ResponseWriter, r *http.Request) {
	// Reserve the single-request budget first so that a request beyond it is
	// rejected deterministically regardless of its body, and every received
	// request is counted.
	s.mu.Lock()
	s.count++
	served := s.count
	s.mu.Unlock()

	if served > 1 {
		w.WriteHeader(s.exhaustedStatus)
		return
	}

	// Bound the body so a client cannot grow the harness memory without bound or
	// stream forever (which would also stall the draining Close). The limit is
	// always applied for a non-nil body; a negative override is treated as zero
	// so WithMaxBodyBytes(0) enforces an empty body rather than bypassing it.
	if r.Body != nil {
		limit := s.maxBodyBytes
		if limit < 0 {
			limit = 0
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
	}

	body, err := readBody(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	captured := captureRequest(r, body)
	s.mu.Lock()
	s.captured = captured
	s.mu.Unlock()

	if err := s.response.write(w); err != nil {
		s.mu.Lock()
		s.writeErr = err
		s.mu.Unlock()
	}
}

// WriteError returns the error, if any, from writing the programmed response
// body to the most recent served request, or nil if the body was written
// without error. Call it after driving the client under test.
func (s *FakeHTTPServer) WriteError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}
