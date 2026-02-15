package connect

import "net/http"

// Service represents a Connect-Go service that can be mounted on a server.
type Service interface {
	// Path returns the HTTP path prefix for the service (e.g. "/foo.v1.FooService/").
	Path() string
	// Handler returns the HTTP handler that serves the service.
	Handler() http.Handler
}

// NewService creates a Service from a path and handler, typically produced by
// a Connect-Go generated constructor (e.g. foov1connect.NewFooServiceHandler).
func NewService(path string, handler http.Handler) Service {
	return &service{path: path, handler: handler}
}

type service struct {
	path    string
	handler http.Handler
}

func (s *service) Path() string         { return s.path }
func (s *service) Handler() http.Handler { return s.handler }
