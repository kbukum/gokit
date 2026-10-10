package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

func requireReason(t *testing.T, err error, code apperrors.ErrorCode, reason string) {
	t.Helper()
	app, ok := apperrors.AsAppError(err)
	if !ok || app.Code != code || app.Reason != reason {
		t.Fatalf("got %v, want %s/%s", err, code, reason)
	}
}

func TestReferenceProtectsTokenGrammar(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := m.Reference(issued.Token)
	if err != nil || ref != issued.Principal.Reference {
		t.Fatalf("reference = %q, %v", ref, err)
	}
	for _, token := range []string{"", strings.Repeat("a", 42), strings.Repeat("!", 43)} {
		_, err := m.Reference(token)
		requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")
	}
}

func TestResolveReturnsAuthoritativeState(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, clock := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Resolve(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation != 1 || first.Family == "" || !first.ExpiresAt.Equal(clock.Now().Add(Lifetime)) ||
		first.Principal.Subject != "u" || first.Principal.Credential != auth.Session || first.Principal.Reference != issued.Principal.Reference {
		t.Fatalf("unexpected state: %+v", first)
	}

	next, err := m.Rotate(context.Background(), issued.Principal.Reference, caller())
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve(context.Background(), issued.Principal.Reference)
	requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")
	second, err := m.Resolve(context.Background(), next.Principal.Reference)
	if err != nil || second.Generation != 2 || second.Family != first.Family || !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("rotated state: %+v %v", second, err)
	}

	clock.Advance(Lifetime)
	_, err = m.Resolve(context.Background(), next.Principal.Reference)
	requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")
	clock.Advance(-Lifetime)

	s.mu.Lock()
	s.fail = errors.New("private database failure")
	s.mu.Unlock()
	_, err = m.Resolve(context.Background(), next.Principal.Reference)
	requireReason(t, err, apperrors.ErrCodeServiceUnavailable, "AUTH_STORE_UNAVAILABLE")
	if strings.Contains(err.Error(), next.Principal.Reference) {
		t.Fatal("error echoes the reference")
	}
	s.mu.Lock()
	s.fail = nil
	s.mu.Unlock()

	if err := m.Logout(context.Background(), next.Principal.Reference); err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve(context.Background(), next.Principal.Reference)
	requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")
	_, err = m.Resolve(context.Background(), "unknown")
	requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")

	if err := m.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = m.Resolve(context.Background(), issued.Principal.Reference)
	requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_CLOSED")
}

func TestResolveHonorsCancellation(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := m.Resolve(ctx, issued.Principal.Reference); err == nil {
		t.Fatal("expired context resolved")
	}
}

func TestResolveReportsAuthenticationTime(t *testing.T) {
	m, clock := fixture(t, &memoryStore{rows: make(map[string]Record)})
	ctx := context.Background()
	signedIn := clock.Now()
	issued, err := m.Create(ctx, caller())
	if err != nil {
		t.Fatal(err)
	}
	state, err := m.Resolve(ctx, issued.Principal.Reference)
	if err != nil || !state.AuthenticatedAt.Equal(signedIn) {
		t.Fatalf("created state: %+v %v", state, err)
	}

	clock.Advance(20 * time.Minute)
	rotated, err := m.Rotate(ctx, issued.Principal.Reference, caller())
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.Resolve(ctx, rotated.Principal.Reference)
	if err != nil || !state.AuthenticatedAt.Equal(signedIn) {
		t.Fatalf("rotation must keep the authentication time: %+v %v", state, err)
	}

	clock.Advance(5 * time.Minute)
	reauthenticated := clock.Now()
	attempt, err := m.BeginLogin(ctx, rotated.Token)
	if err != nil {
		t.Fatal(err)
	}
	relogged, err := m.CompleteLogin(ctx, attempt, caller())
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.Resolve(ctx, relogged.Principal.Reference)
	if err != nil || !state.AuthenticatedAt.Equal(reauthenticated) || !state.ExpiresAt.Equal(signedIn.Add(Lifetime)) {
		t.Fatalf("relogin must record the new authentication and keep the expiry: %+v %v", state, err)
	}
}
