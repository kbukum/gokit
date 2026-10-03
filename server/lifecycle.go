package server

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Quiesce marks readiness false and rejects new public requests before waiting for existing work.
func (s *Server) Quiesce() error {
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	if !s.closing {
		s.closing = true
		if s.active == 0 {
			close(s.drained)
		}
	}
	return nil
}

func (s *Server) admit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.admissionMu.Lock()
		if s.closing {
			s.admissionMu.Unlock()
			http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.active++
		s.admissionMu.Unlock()
		defer func() {
			s.admissionMu.Lock()
			defer s.admissionMu.Unlock()
			s.active--
			if s.closing && s.active == 0 {
				close(s.drained)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) shutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := time.Duration(s.config.ShutdownTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func remainingBudget(ctx context.Context) time.Duration {
	deadline, _ := ctx.Deadline()
	return max(time.Until(deadline), 0)
}

func (s *Server) drain(ctx context.Context) error {
	if err := s.Quiesce(); err != nil {
		return err
	}
	ctx, cancel := s.shutdownContext(ctx)
	defer cancel()
	// Reserve one fifth of this phase for force-close and cooperative handler teardown.
	graceCtx, graceCancel := context.WithTimeout(ctx, remainingBudget(ctx)*4/5)
	err := s.httpServer.Shutdown(graceCtx)
	graceCancel()
	if err != nil {
		err = errors.Join(err, s.httpServer.Close())
	}
	if s.cancelRequests != nil {
		s.cancelRequests()
	}
	select {
	case <-s.drained:
	case <-ctx.Done():
		err = errors.Join(err, ctx.Err())
	}
	return err
}
