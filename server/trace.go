package server

import "github.com/kbukum/gokit/server/middleware"

// Trace enables HTTP tracing with a composition-owned tracer. ApplyMiddleware installs it inside recovery and request IDs and outside request logging, so every request log line and problem response carries the span's trace ID. Configure before ApplyMiddleware or ApplyDefaults.
func (s *Server) Trace(cfg middleware.TracingConfig) error {
	tracing, err := middleware.Tracing(cfg)
	if err != nil {
		return err
	}
	s.tracingMW = tracing
	return nil
}
