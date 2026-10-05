package session

import (
	"context"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// SignIn is a validated login plus the session cookie the browser presented, which is empty for a fresh login.
type SignIn struct {
	Login     Login
	Presented string
}

// SignOut is a validated session cookie plus the synchronizer CSRF token presented with it.
type SignOut struct {
	Token     string
	CSRFToken string
}

// Grant is an authoritative session view. Token is set only by SignIn.
type Grant struct {
	Token     string
	Principal auth.Principal
	CSRFToken string
}

// Backend owns session state behind the browser handler. Implementations return typed failures. The handler passes
// backend errors, causes included, to HandlerConfig.Errors, which must normalize them so causes never reach the browser.
type Backend interface {
	SignIn(context.Context, SignIn) (Grant, error)
	Status(ctx context.Context, token string) (Grant, error)
	SignOut(context.Context, SignOut) error
}

type localBackend struct {
	manager  *Manager
	verifier LoginVerifier
}

// NewLocalBackend serves sessions from manager, verifying passwords with the application-owned verifier.
func NewLocalBackend(manager *Manager, verifier LoginVerifier) (Backend, error) {
	if manager == nil || util.IsNil(verifier) {
		return nil, apperrors.InvalidInput("auth", "Manager and verifier are required")
	}
	return &localBackend{manager: manager, verifier: verifier}, nil
}

// SignIn begins the attempt before verifying, so an invalid presented cookie fails without spending password work.
func (b *localBackend) SignIn(ctx context.Context, in SignIn) (Grant, error) {
	attempt, err := b.manager.BeginLogin(ctx, in.Presented)
	if err != nil {
		return Grant{}, err
	}
	p, err := b.verifier.VerifyLogin(ctx, in.Login)
	if err != nil {
		if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeUnauthorized {
			return Grant{}, auth.Failure("LOGIN_INVALID")
		}
		return Grant{}, storeFailure(err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Grant{}, storeFailure(ctxErr)
	}
	issued, err := b.manager.CompleteLogin(ctx, attempt, p)
	if err != nil {
		return Grant{}, err
	}
	csrf, err := b.manager.CSRFToken(ctx, issued.Principal.Reference)
	if err != nil {
		return Grant{}, err
	}
	return Grant{Token: issued.Token, Principal: issued.Principal, CSRFToken: csrf}, nil
}

func (b *localBackend) Status(ctx context.Context, token string) (Grant, error) {
	if err := ValidateToken(token); err != nil {
		return Grant{}, err
	}
	row, err := b.manager.lookup(ctx, b.manager.protection.Digest(token))
	if err != nil {
		return Grant{}, err
	}
	csrf, err := b.manager.csrf.Issue(row.Reference)
	if err != nil {
		return Grant{}, err
	}
	return Grant{Principal: row.Principal.Clone(), CSRFToken: csrf}, nil
}

func (b *localBackend) SignOut(ctx context.Context, in SignOut) error {
	if err := ValidateToken(in.Token); err != nil {
		return err
	}
	ref := b.manager.protection.Digest(in.Token)
	if err := b.manager.csrf.Verify(ref, in.CSRFToken); err != nil {
		return err
	}
	return b.manager.Logout(ctx, ref)
}
