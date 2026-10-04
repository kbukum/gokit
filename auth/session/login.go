package session

import (
	"context"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

// LoginAttempt is the presented cookie's state captured by BeginLogin before password verification.
// The zero value is invalid; an attempt is bound to the Manager that began it.
type LoginAttempt struct {
	manager *Manager
	token   string
	state   attemptState
}

type attemptState uint8

const (
	attemptInvalid attemptState = iota
	// attemptFresh: no cookie, or a canonical cookie unknown to the store.
	attemptFresh
	// attemptActive: an active generation that a committed logout must still be able to stop.
	attemptActive
	// attemptTerminal: a generation that was already revoked or superseded when login began.
	attemptTerminal
)

// BeginLogin classifies the presented cookie before the application verifies the password, so that a logout committed
// during verification still wins. An empty token begins a fresh login. Malformed tokens, corrupt records and store
// failures are rejected before any password work.
func (m *Manager) BeginLogin(ctx context.Context, token string) (LoginAttempt, error) {
	if token == "" {
		if err := m.ready(); err != nil {
			return LoginAttempt{}, err
		}
		return LoginAttempt{manager: m, state: attemptFresh}, nil
	}
	if err := ValidateToken(token); err != nil {
		return LoginAttempt{}, err
	}
	if err := m.ready(); err != nil {
		return LoginAttempt{}, err
	}
	ref := m.protection.Digest(token)
	ctx, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	row, err := m.store.Lookup(ctx, ref)
	if ctx.Err() != nil {
		return LoginAttempt{}, storeFailure(ctx.Err())
	}
	if err != nil {
		if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeNotFound {
			return LoginAttempt{manager: m, token: token, state: attemptFresh}, nil
		}
		return LoginAttempt{}, storeFailure(err)
	}
	if corrupt(row, ref) {
		return LoginAttempt{}, auth.Failure("SESSION_INVALID")
	}
	if !row.Active || row.Revoked {
		return LoginAttempt{manager: m, token: token, state: attemptTerminal}, nil
	}
	return LoginAttempt{manager: m, token: token, state: attemptActive}, nil
}

// CompleteLogin issues a credential for a principal the application verified after BeginLogin.
// A fresh attempt creates a family. An active attempt atomically replaces its generation, preserving an unexpired
// family's absolute expiry, and fails if the family was revoked or superseded meanwhile. A terminal attempt revokes the
// stale cookie's whole family before issuing a fresh family, so a browser stuck with a stale HttpOnly cookie recovers
// through strong authentication while every generation of the old family stays fenced.
func (m *Manager) CompleteLogin(ctx context.Context, attempt LoginAttempt, p auth.Principal) (Issued, error) {
	if attempt.manager != m || attempt.state == attemptInvalid {
		return Issued{}, apperrors.InvalidInput("login", "Login attempt was not begun by this manager")
	}
	ctx, cancel := context.WithTimeout(ctx, MutationBudget)
	defer cancel()
	return admitted(ctx, m, func() (Issued, error) {
		switch attempt.state {
		case attemptTerminal:
			return m.replaceTerminal(ctx, m.protection.Digest(attempt.token), p)
		case attemptActive:
			return m.relogin(ctx, m.protection.Digest(attempt.token), p)
		default:
			return m.create(ctx, p)
		}
	})
}

func corrupt(row Record, ref string) bool {
	return row.Reference != ref || row.Family == "" || row.Generation == 0 || row.ExpiresAt.IsZero() || row.RetainUntil.Before(row.ExpiresAt.Add(Retention))
}

func (m *Manager) relogin(ctx context.Context, ref string, p auth.Principal) (Issued, error) {
	readCtx, readCancel := context.WithTimeout(ctx, LookupBudget)
	defer readCancel()
	row, err := m.store.Lookup(readCtx, ref)
	if readCtx.Err() != nil {
		return Issued{}, storeFailure(readCtx.Err())
	}
	if err != nil {
		return Issued{}, storeFailure(err)
	}
	if corrupt(row, ref) || !row.Active || row.Revoked {
		return Issued{}, auth.Failure("SESSION_INVALID")
	}
	token, err := m.newToken()
	if err != nil {
		return Issued{}, err
	}
	p = p.Clone()
	p.Reference = m.protection.Digest(token)
	p.Credential = auth.Session
	p.ExpiresAt = row.ExpiresAt
	if now := m.clock.Now(); !now.Before(p.ExpiresAt) {
		p.ExpiresAt = now.Add(Lifetime)
	}
	if err := p.Validate(); err != nil {
		return Issued{}, err
	}
	next := Record{Reference: p.Reference, Family: row.Family, Generation: row.Generation + 1, Principal: p, ExpiresAt: p.ExpiresAt, RetainUntil: p.ExpiresAt.Add(Retention), Active: true}
	if err := m.store.Relogin(ctx, ref, next); err != nil {
		return Issued{}, storeFailure(err)
	}
	m.cancelFamily(row.Family)
	return Issued{Token: token, Principal: p.Clone()}, nil
}

// replaceTerminal revokes the terminal credential's family, cancels its local streams, then issues a fresh family.
func (m *Manager) replaceTerminal(ctx context.Context, ref string, p auth.Principal) (Issued, error) {
	family, err := m.store.Revoke(ctx, ref)
	if err != nil {
		return Issued{}, storeFailure(err)
	}
	m.cancelFamily(family)
	return m.create(ctx, p)
}
