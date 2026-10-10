package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/session"
	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

func fixture(t *testing.T) (*store, *dbkit.DB, *util.FakeClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join("testdata", ".runtime", fmt.Sprintf("%d-%s.db", os.Getpid(), t.Name()))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Error(err)
		}
	})
	db, err := dbkit.NewWithContext(ctx, sqlite.Dialect(), dbkit.Config{DSN: path, LogLevel: "silent"}, logging.NewDefault("session-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg := Migrations(db, sqlite.MigrateDriver())
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Ready(ctx, SchemaVersion); err != nil {
		t.Fatal(err)
	}
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	s, err := NewStore(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	return s.(*store), db, clock
}

func row(ref, family string, clock util.Clock) session.Record {
	return session.Record{Reference: ref, Family: family, Generation: 1, Principal: auth.Principal{Subject: "u", Kind: auth.User, Credential: auth.Session, Reference: ref, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}, ExpiresAt: clock.Now().Add(time.Hour), RetainUntil: clock.Now().Add(time.Hour + 10*time.Minute), AuthenticatedAt: clock.Now(), Active: true}
}

func TestProductionMigrationRotationRevocation(t *testing.T) {
	s, db, clock := fixture(t)
	ctx := context.Background()
	cfg := Migrations(db, sqlite.MigrateDriver())
	if err := cfg.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, row("old", "family", clock)); err != nil {
		t.Fatal(err)
	}
	next := row("new", "family", clock)
	next.Generation = 2
	if err := s.Rotate(ctx, "old", next); err != nil {
		t.Fatal(err)
	}
	old, err := s.Lookup(ctx, "old")
	if err != nil || old.Active {
		t.Fatal(err, old)
	}
	if _, err := s.Revoke(ctx, "old"); err != nil {
		t.Fatal(err)
	}
	current, err := s.Lookup(ctx, "new")
	if err != nil || !current.Revoked {
		t.Fatal(err, current)
	}
	later := next
	later.Reference = "later"
	later.Generation++
	if err := s.Rotate(ctx, "new", later); err == nil {
		t.Fatal("resurrected")
	}
	if _, err := s.Revoke(ctx, "old"); err != nil {
		t.Fatal("logout not idempotent", err)
	}
	if _, err := s.Lookup(ctx, "missing"); err == nil {
		t.Fatal("missing accepted")
	}
	if _, err := s.Cleanup(ctx, clock.Now(), 256); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(ctx, "old"); err != nil {
		t.Fatal("early tombstone deletion", err)
	}
	clock.Advance(time.Hour + 10*time.Minute)
	n, err := s.Cleanup(ctx, clock.Now(), 256)
	if err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if _, err := s.Lookup(ctx, "old"); err == nil {
		t.Fatal("tombstone remained")
	}
	if err := cfg.Ready(ctx, SchemaVersion); err != nil {
		t.Fatal("cleanup altered schema", err)
	}
}

func TestConcurrentRotationCAS(t *testing.T) {
	s, _, clock := fixture(t)
	ctx := context.Background()
	if err := s.Create(ctx, row("old", "family", clock)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, ref := range []string{"a", "b"} {
		wg.Go(func() { next := row(ref, "family", clock); next.Generation = 2; results <- s.Rotate(ctx, "old", next) })
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("CAS wins=%d", wins)
	}
}

func TestCanceledTransactionAndSchemaReadiness(t *testing.T) {
	s, db, clock := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Create(ctx, row("ref", "family", clock)); err == nil {
		t.Fatal("canceled write")
	}
	if _, err := s.Lookup(context.Background(), "ref"); err == nil {
		t.Fatal("canceled write committed")
	}
	cfg := Migrations(db, sqlite.MigrateDriver())
	if err := cfg.Ready(context.Background(), SchemaVersion+1); err == nil {
		t.Fatal("wrong schema accepted")
	}
	if err := cfg.Down(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(context.Background(), "ref"); err == nil {
		t.Fatal("missing schema accepted")
	}
}

func TestStoreRejectsInvalidTransitionsAndRollsBack(t *testing.T) {
	s, db, clock := fixture(t)
	ctx := context.Background()
	for _, r := range []session.Record{{}, row("ref", "family", clock)} {
		if r.Reference != "" {
			r.Principal.Kind = ""
		}
		if err := s.Create(ctx, r); err == nil {
			t.Fatal("invalid row")
		}
	}
	if _, err := NewStore(nil); err == nil {
		t.Fatal("missing database")
	}
	if _, err := NewStore(db, nil); err == nil {
		t.Fatal("missing clock")
	}
	var typedNil *util.FakeClock
	if _, err := NewStore(db, typedNil); err == nil {
		t.Fatal("typed-nil clock")
	}
	if _, err := NewStore(db, clock, clock); err == nil {
		t.Fatal("ambiguous clock")
	}
	if err := s.Create(ctx, row("old", "family", clock)); err != nil {
		t.Fatal(err)
	}
	duplicate := row("old", "other", clock)
	if err := s.Create(ctx, duplicate); err == nil {
		t.Fatal("duplicate credential accepted")
	}
	var count int64
	if err := db.WithContext(ctx).Model(&family{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("create not atomic", count, err)
	}
	next := row("new", "family", clock)
	next.Generation = 2
	next.Family = "wrong"
	if err := s.Rotate(ctx, "old", next); err == nil {
		t.Fatal("wrong family accepted")
	}
	next.Family = "family"
	next.ExpiresAt = next.ExpiresAt.Add(time.Hour)
	next.RetainUntil = next.ExpiresAt.Add(session.Retention)
	if err := s.Rotate(ctx, "old", next); err == nil {
		t.Fatal("expiry extension accepted")
	}
	next = row("old", "family", clock)
	next.Generation = 2
	if err := s.Rotate(ctx, "old", next); err == nil {
		t.Fatal("credential reuse accepted")
	}
	current, err := s.Lookup(ctx, "old")
	if err != nil || !current.Active || current.Generation != 1 {
		t.Fatal("failed CAS not rolled back", current, err)
	}
	next = row("new", "family", clock)
	next.Generation = 2
	clock.Advance(time.Hour)
	if err := s.Rotate(ctx, "old", next); err == nil {
		t.Fatal("expired rotation accepted")
	}
	for _, limit := range []int{0, 257} {
		if _, err := s.Cleanup(ctx, clock.Now(), limit); err == nil {
			t.Fatal("invalid batch")
		}
	}
	if _, err := s.Revoke(ctx, "missing"); err == nil {
		t.Fatal("unknown revocation")
	}
}

func TestRevokeSubjectRevokesOnlyThatSubjectsActiveFamilies(t *testing.T) {
	s, _, clock := fixture(t)
	ctx := context.Background()
	alice := func(ref, family string) session.Record {
		r := row(ref, family, clock)
		r.Principal.Subject = "alice"
		return r
	}
	for _, r := range []session.Record{alice("a1", "fa1"), alice("a2", "fa2"), row("u1", "fu1", clock)} {
		if err := s.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	service := alice("s1", "fs1")
	service.Principal.Kind = auth.Service
	if err := s.Create(ctx, service); err != nil {
		t.Fatal(err)
	}
	rotated := alice("a3", "fa1")
	rotated.Generation = 2
	if err := s.Rotate(ctx, "a1", rotated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Revoke(ctx, "a2"); err != nil {
		t.Fatal(err)
	}

	families, err := s.RevokeSubject(ctx, auth.User, "alice")
	if err != nil || families != 1 {
		t.Fatalf("families = %v, %v", families, err)
	}
	for ref, revoked := range map[string]bool{"a1": true, "a3": true, "a2": true, "u1": false, "s1": false} {
		got, err := s.Lookup(ctx, ref)
		if err != nil || got.Revoked != revoked {
			t.Fatalf("%s: revoked=%v err=%v", ref, got.Revoked, err)
		}
	}
	again, err := s.RevokeSubject(ctx, auth.User, "alice")
	if err != nil || again != 0 {
		t.Fatalf("again = %v, %v", again, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.RevokeSubject(canceled, auth.User, "bob"); err == nil {
		t.Fatal("canceled revocation succeeded")
	}
}

func TestStoreKeepsAuthenticationTime(t *testing.T) {
	s, db, clock := fixture(t)
	ctx := context.Background()
	first := row("first", "family", clock)
	signedIn := first.AuthenticatedAt
	if err := s.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err := s.Lookup(ctx, "first")
	if err != nil || !got.AuthenticatedAt.Equal(signedIn) {
		t.Fatalf("created: %+v %v", got, err)
	}

	clock.Advance(time.Minute)
	rotated := row("rotated", "family", clock)
	rotated.Generation, rotated.ExpiresAt, rotated.RetainUntil = 2, first.ExpiresAt, first.RetainUntil
	if err := s.Rotate(ctx, "first", rotated); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Lookup(ctx, "rotated"); err != nil || !got.AuthenticatedAt.Equal(signedIn) {
		t.Fatalf("rotation must keep the authentication time: %+v %v", got, err)
	}

	clock.Advance(time.Minute)
	relogged := row("relogged", "family", clock)
	relogged.Generation, relogged.ExpiresAt, relogged.RetainUntil = 3, first.ExpiresAt, first.RetainUntil
	if err := s.Relogin(ctx, "rotated", relogged); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Lookup(ctx, "relogged"); err != nil || !got.AuthenticatedAt.Equal(clock.Now()) {
		t.Fatalf("relogin must record the new authentication time: %+v %v", got, err)
	}

	unknown := row("unknown", "unknown-family", clock)
	unknown.AuthenticatedAt = time.Time{}
	if err := s.Create(ctx, unknown); err == nil {
		t.Fatal("a session without an authentication time was created")
	}
	if err := db.GormDB.WithContext(ctx).Exec("UPDATE auth_session_families SET authenticated_at = NULL WHERE id = ?", "family").Error; err != nil {
		t.Fatal(err)
	}
	if got, err = s.Lookup(ctx, "relogged"); err != nil || !got.AuthenticatedAt.IsZero() {
		t.Fatalf("a missing authentication time must read as zero: %+v %v", got, err)
	}
}
