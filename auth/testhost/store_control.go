package testhost

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/kbukum/gokit/auth/session"
	apperrors "github.com/kbukum/gokit/errors"
)

// controlledStore injects a declared fixture-only storage outage without replacing the production SQL adapter.
type controlledStore struct {
	session.Store
	unavailable atomic.Bool
}

func (s *controlledStore) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.unavailable.Load() {
		return apperrors.New(apperrors.ErrCodeServiceUnavailable, "Authentication store unavailable").WithReason("AUTH_STORE_UNAVAILABLE")
	}
	return nil
}

func (s *controlledStore) Create(ctx context.Context, record session.Record) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	return s.Store.Create(ctx, record)
}

func (s *controlledStore) Lookup(ctx context.Context, reference string) (session.Record, error) {
	if err := s.check(ctx); err != nil {
		return session.Record{}, err
	}
	return s.Store.Lookup(ctx, reference)
}

func (s *controlledStore) Rotate(ctx context.Context, reference string, record session.Record) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	return s.Store.Rotate(ctx, reference, record)
}

func (s *controlledStore) Relogin(ctx context.Context, reference string, record session.Record) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	return s.Store.Relogin(ctx, reference, record)
}

func (s *controlledStore) Revoke(ctx context.Context, reference string) (string, error) {
	if err := s.check(ctx); err != nil {
		return "", err
	}
	return s.Store.Revoke(ctx, reference)
}

func (s *controlledStore) Cleanup(ctx context.Context, now time.Time, limit int) (int64, error) {
	if err := s.check(ctx); err != nil {
		return 0, err
	}
	return s.Store.Cleanup(ctx, now, limit)
}
