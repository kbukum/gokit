package database

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/session"
	dbkit "github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/database/sqlite"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// instance builds an independent manager over its own database handle, as a second process would.
func instance(t *testing.T, db *dbkit.DB, clock util.Clock) *session.Manager {
	t.Helper()
	s, err := NewStore(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := security.NewSignedCSRF(bytes.Repeat([]byte{7}, 32), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m, err := session.NewManager(session.Config{Store: s, Clock: clock, Random: rand.Reader, Pepper: strings.Repeat("p", 32), CSRF: csrf, ReportError: func(context.Context, error) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := m.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}

func cookieRequest(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://example.test/", http.NoBody)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	return r
}

// assertCrossInstanceRevocation proves that logout on one instance is authoritative for another sharing the database.
func assertCrossInstanceRevocation(t *testing.T, first, second *dbkit.DB, clock util.Clock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := instance(t, first, clock), instance(t, second, clock)
	issued, err := a.Create(ctx, auth.Principal{Subject: "u", Kind: auth.User, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := b.Authenticate(cookieRequest(issued.Token))
	if err != nil {
		t.Fatal("second instance rejected a live session", err)
	}
	lifetime, release, err := b.Acquire(ctx, p.Reference)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	if err := a.Logout(ctx, issued.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Authenticate(cookieRequest(issued.Token)); err == nil {
		t.Fatal("second instance accepted a request after revocation")
	}
	b.Nudge()
	select {
	case <-lifetime.Done():
	case <-ctx.Done():
		t.Fatal("second instance retained a revoked stream after revalidation")
	}
	if _, err := a.Authenticate(cookieRequest(issued.Token)); err == nil {
		t.Fatal("revoking instance resurrected the session")
	}
}

func TestSQLiteCrossInstanceRevocation(t *testing.T) {
	_, first, clock := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var path string
	if err := first.WithContext(ctx).Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&path).Error; err != nil || path == "" {
		t.Fatal("resolve database file", err)
	}
	second, err := dbkit.NewWithContext(ctx, sqlite.Dialect(), dbkit.Config{DSN: path, LogLevel: "silent"}, logging.NewDefault("session-test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	})
	assertCrossInstanceRevocation(t, first, second, clock)
}
