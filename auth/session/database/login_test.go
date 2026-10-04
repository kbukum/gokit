package database

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/session"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestSQLReloginRecoveryRetentionAndRollback(t *testing.T) {
	s, _, clock := fixture(t)
	ctx := context.Background()
	first := row("old", "family", clock)
	if err := s.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	next := first
	next.Reference = "new"
	next.Principal.Reference = next.Reference
	next.Principal.Subject = "other-user"
	next.Generation++
	if err := s.Relogin(ctx, first.Reference, next); err != nil {
		t.Fatal(err)
	}
	if err := s.Relogin(ctx, first.Reference, next); err == nil {
		t.Fatal("stale generation recovered")
	}
	current, err := s.Lookup(ctx, next.Reference)
	if err != nil || !current.Active || current.Principal.Subject != "other-user" {
		t.Fatal("strong-auth identity replacement failed", err)
	}
	clock.Advance(time.Hour)
	recovered := row("recovered", "family", clock)
	recovered.Generation = 3
	if err := s.Relogin(ctx, next.Reference, recovered); err != nil {
		t.Fatal("expired active credential failed recovery", err)
	}
	clock.Advance(session.Retention)
	if n, err := s.Cleanup(ctx, clock.Now(), 256); err != nil || n != 0 {
		t.Fatal("recovery lost family tombstones", n, err)
	}
	if err := s.Create(ctx, row("collision", "other-family", clock)); err != nil {
		t.Fatal(err)
	}
	conflict := recovered
	conflict.Reference = "collision"
	conflict.Principal.Reference = conflict.Reference
	conflict.Generation++
	if err := s.Relogin(ctx, recovered.Reference, conflict); err == nil {
		t.Fatal("duplicate successor committed")
	}
	current, err = s.Lookup(ctx, recovered.Reference)
	if err != nil || !current.Active || current.Generation != recovered.Generation {
		t.Fatal("failed relogin did not roll back", err)
	}
	extended := recovered
	extended.Reference = "extended"
	extended.Principal.Reference = extended.Reference
	extended.Generation++
	extended.ExpiresAt = recovered.ExpiresAt.Add(time.Second)
	extended.RetainUntil = extended.ExpiresAt.Add(session.Retention)
	if err := s.Relogin(ctx, recovered.Reference, extended); err == nil {
		t.Fatal("unexpired relogin extended absolute expiry")
	}
	extended.ExpiresAt = clock.Now().Add(2 * session.Lifetime)
	extended.RetainUntil = extended.ExpiresAt.Add(session.Retention)
	if err := s.Relogin(ctx, recovered.Reference, extended); err == nil {
		t.Fatal("unbounded lifetime accepted")
	}
	if _, err := s.Revoke(ctx, first.Reference); err != nil {
		t.Fatal(err)
	}
	current, err = s.Lookup(ctx, recovered.Reference)
	if err != nil || !current.Revoked {
		t.Fatal("old generation failed to revoke recovered family", err)
	}
	extended.ExpiresAt = recovered.ExpiresAt
	extended.RetainUntil = recovered.RetainUntil
	if err := s.Relogin(ctx, recovered.Reference, extended); err == nil {
		t.Fatal("revoked family resurrected")
	}
}

func TestSQLConcurrentReloginAndLogout(t *testing.T) {
	s, _, clock := fixture(t)
	ctx := context.Background()
	first := row("old", "family", clock)
	if err := s.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	next := first
	next.Reference = "new"
	next.Principal.Reference = next.Reference
	next.Generation++
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := s.Relogin(ctx, first.Reference, next); err != nil && apperrors.Normalize(err).Code != apperrors.ErrCodeUnauthorized {
			t.Error(err)
		}
	})
	wg.Go(func() {
		if _, err := s.Revoke(ctx, first.Reference); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	old, err := s.Lookup(ctx, first.Reference)
	if err != nil || !old.Revoked {
		t.Fatal("logout failed", err)
	}
	current, err := s.Lookup(ctx, next.Reference)
	if err == nil && !current.Revoked {
		t.Fatal("concurrent relogin resurrected committed logout")
	}
}
