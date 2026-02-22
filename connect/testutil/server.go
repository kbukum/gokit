package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

var _ component.Component = (*Server)(nil)
var _ testutil.TestComponent = (*Server)(nil)

// Server is a test server for ConnectRPC handlers backed by httptest.Server.
// It implements both component.Component and testutil.TestComponent.
//
// Mount Connect handlers before starting the server, then use BaseURL()
// to create real ConnectRPC clients for testing.
type Server struct {
	mux     *http.ServeMux
	ts      *httptest.Server
	started bool
	mu      sync.RWMutex
}

// NewServer creates a new test Connect server.
func NewServer() *Server {
	return &Server{
		mux: http.NewServeMux(),
	}
}

// Mount registers a ConnectRPC handler on the test server.
// Call this before Start() with the path and handler returned by
// a generated New*Handler function.
//
// Example:
//
//	path, handler := mypbconnect.NewMyServiceHandler(&impl{})
//	srv.Mount(path, handler)
func (s *Server) Mount(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// BaseURL returns the test server's base URL (e.g., "http://127.0.0.1:PORT").
// Returns empty string if the server is not started.
func (s *Server) BaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ts == nil {
		return ""
	}
	return s.ts.URL
}

// Client returns an *http.Client configured for the test server.
// Use this with ConnectRPC client constructors.
func (s *Server) Client() *http.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ts == nil {
		return http.DefaultClient
	}
	return s.ts.Client()
}

// --- component.Component ---

func (s *Server) Name() string { return "connect-test-server" }

func (s *Server) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("connect test server already started")
	}

	s.ts = httptest.NewUnstartedServer(s.mux)
	s.ts.EnableHTTP2 = true
	s.ts.StartTLS()
	s.started = true
	return nil
}

func (s *Server) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.ts == nil {
		return nil
	}
	s.ts.Close()
	s.ts = nil
	s.started = false
	return nil
}

func (s *Server) Health(_ context.Context) component.Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.started {
		return component.Health{Name: s.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: s.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

// Reset stops the existing server and creates a new one with a fresh mux.
func (s *Server) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ts != nil {
		s.ts.Close()
	}
	s.mux = http.NewServeMux()
	s.started = false
	return nil
}

// Snapshot is a no-op (servers are stateless).
func (s *Server) Snapshot(_ context.Context) (interface{}, error) {
	return nil, nil
}

// Restore is a no-op.
func (s *Server) Restore(_ context.Context, _ interface{}) error {
	return nil
}
