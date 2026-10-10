package session

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/lease"
	apperrors "github.com/kbukum/gokit/errors"
)

func principal(kind auth.Kind, subject string) auth.Principal {
	return auth.Principal{Subject: subject, Kind: kind, Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}
}

func TestRevokeSubjectEndsEveryFamilyOfThatSubjectOnly(t *testing.T) {
	m, _ := fixture(t, &memoryStore{rows: make(map[string]Record)})
	ctx := context.Background()
	first, err := m.Create(ctx, principal(auth.User, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := m.Rotate(ctx, first.Principal.Reference, principal(auth.User, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Create(ctx, principal(auth.User, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.Create(ctx, principal(auth.User, "bob"))
	if err != nil {
		t.Fatal(err)
	}
	sameSubjectService, err := m.Create(ctx, principal(auth.Service, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	life, release, err := m.Acquire(ctx, second.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	n, err := m.RevokeSubject(ctx, auth.User, "alice")
	if err != nil || n != 2 {
		t.Fatalf("revoked %d families, %v", n, err)
	}
	for _, ref := range []string{rotated.Principal.Reference, second.Principal.Reference} {
		_, err := m.Resolve(ctx, ref)
		requireReason(t, err, apperrors.ErrCodeUnauthorized, "SESSION_INVALID")
	}
	if !errors.Is(context.Cause(life), lease.ErrRevoked) {
		t.Fatalf("stream cause = %v", context.Cause(life))
	}
	for _, ref := range []string{other.Principal.Reference, sameSubjectService.Principal.Reference} {
		if _, err := m.Resolve(ctx, ref); err != nil {
			t.Fatalf("unrelated session ended: %v", err)
		}
	}

	again, err := m.RevokeSubject(ctx, auth.User, "alice")
	if err != nil || again != 0 {
		t.Fatalf("second revocation = %d, %v", again, err)
	}
}

func TestRevokeSubjectValidatesAndReportsStoreFailure(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	ctx := context.Background()
	for _, tc := range []struct {
		kind    auth.Kind
		subject string
	}{{auth.User, ""}, {"robot", "alice"}} {
		_, err := m.RevokeSubject(ctx, tc.kind, tc.subject)
		requireReason(t, err, apperrors.ErrCodeInvalidInput, "")
	}
	s.fail = errors.New("store down")
	_, err := m.RevokeSubject(ctx, auth.User, "alice")
	if app, ok := apperrors.AsAppError(err); !ok || app.Code != apperrors.ErrCodeServiceUnavailable {
		t.Fatalf("store failure = %v", err)
	}
}

func TestSubjectRevokeZeroCountStillEndsLocalWatch(t *testing.T) {
	s := &memoryStore{rows: make(map[string]Record)}
	m, _ := fixture(t, s)
	issued, err := m.Create(context.Background(), caller())
	if err != nil {
		t.Fatal(err)
	}
	life, release, err := m.Acquire(context.Background(), issued.Principal.Reference)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := s.RevokeSubject(context.Background(), auth.User, issued.Principal.Subject); err != nil {
		t.Fatal(err)
	}
	if count, err := m.RevokeSubject(context.Background(), auth.User, issued.Principal.Subject); err != nil || count != 0 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if life.Err() == nil {
		t.Fatal("zero-count revocation retained a local stream")
	}
}
