package server

import (
	"errors"

	"github.com/kbukum/gokit/server/middleware"
)

// Trace enables HTTP tracing with a composition-owned tracer. ApplyMiddleware installs it inside recovery and request IDs and outside request logging, so every request log line and problem response carries the span's trace ID. Configure before ApplyMiddleware or ApplyDefaults.
func (s *Server) Trace(cfg middleware.TracingConfig) error {
	tracing, err := middleware.Tracing(cfg)
	if err != nil {
		return err
	}
	s.tracingMW = tracing
	return nil
}

// Authenticate installs authentication that runs once per request just outside tracing, such as [middleware.AuthStages.Authenticate], so its outcome can decide trace trust ([middleware.AuthStages.Trusted]) and access ([middleware.AuthStages.Enforce] around the protected handlers). It must not write responses. Configure before ApplyMiddleware or ApplyDefaults.
func (s *Server) Authenticate(authenticate middleware.Middleware) error {
	if authenticate == nil {
		return errors.New("server: authentication middleware is required")
	}
	s.authMW = authenticate
	return nil
}
