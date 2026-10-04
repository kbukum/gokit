package auth

import (
	"context"
	"net/http"
	"slices"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Kind distinguishes human and service identities.
type Kind string

const (
	User    Kind = "user"
	Service Kind = "service"
)

// Credential identifies the authentication mechanism, never its plaintext.
type Credential string

const (
	Session Credential = "session"
	APIKey  Credential = "apikey"
	Bearer  Credential = "bearer"
)

// RestrictionMode distinguishes an unrestricted credential from an empty deny-all ceiling.
type RestrictionMode string

const (
	Unrestricted RestrictionMode = "unrestricted"
	Restricted   RestrictionMode = "restricted"
)

// Restrictions are credential ceilings, not resource membership grants.
type Restrictions struct {
	Mode      RestrictionMode `json:"mode"`
	Resources []string        `json:"resources,omitempty"`
	Scopes    []string        `json:"scopes,omitempty"`
}

// Principal is the common authenticated caller. Reference is a protected credential reference.
type Principal struct {
	Subject      string       `json:"subject"`
	Kind         Kind         `json:"kind"`
	Credential   Credential   `json:"-"`
	Reference    string       `json:"-"`
	ExpiresAt    time.Time    `json:"-"`
	Restrictions Restrictions `json:"restrictions"`
}

// Clone prevents mutation of shared credential restrictions.
func (p Principal) Clone() Principal {
	p.Restrictions.Resources = slices.Clone(p.Restrictions.Resources)
	p.Restrictions.Scopes = slices.Clone(p.Restrictions.Scopes)
	return p
}

// Validate rejects incomplete identities and ambiguous ceilings.
func (p Principal) Validate() error {
	if p.Subject == "" || (p.Kind != User && p.Kind != Service) ||
		(p.Credential != Session && p.Credential != APIKey && p.Credential != Bearer) || p.Reference == "" {
		return Failure("INVALID_IDENTITY")
	}
	return p.Restrictions.Validate()
}

// Validate rejects unknown modes and restrictions attached to unrestricted credentials.
func (r Restrictions) Validate() error {
	if r.Mode != Restricted && r.Mode != Unrestricted {
		return Failure("INVALID_RESTRICTIONS")
	}
	if r.Mode == Unrestricted && (len(r.Resources) != 0 || len(r.Scopes) != 0) {
		return Failure("INVALID_RESTRICTIONS")
	}
	for _, values := range [][]string{r.Resources, r.Scopes} {
		for _, value := range values {
			if value == "" {
				return Failure("INVALID_RESTRICTIONS")
			}
		}
	}
	return nil
}

// Allows checks only the credential ceiling; composition must separately authorize membership.
func (p Principal) Allows(resource string, scopes ...string) bool {
	if p.Restrictions.Validate() != nil {
		return false
	}
	if p.Restrictions.Mode == Unrestricted {
		return true
	}
	if !slices.Contains(p.Restrictions.Resources, resource) {
		return false
	}
	for _, scope := range scopes {
		if !slices.Contains(p.Restrictions.Scopes, scope) {
			return false
		}
	}
	return true
}

// Policy supplies application-owned authorization, independent of credential ceilings.
type Policy interface {
	Authorize(context.Context, Principal, string, []string) error
}

// Authorize intersects application authorization with the credential ceiling.
func Authorize(ctx context.Context, policy Policy, p Principal, resource string, scopes ...string) error {
	if p.Validate() != nil || !p.Allows(resource, scopes...) || util.IsNil(policy) {
		return apperrors.New(apperrors.ErrCodeForbidden, "Permission denied").WithReason("CREDENTIAL_CEILING")
	}
	return policy.Authorize(ctx, p.Clone(), resource, slices.Clone(scopes))
}

// RequestAuthenticator resolves exactly one request credential.
type RequestAuthenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

// RequestAuthenticatorFunc adapts a request authenticator.
type RequestAuthenticatorFunc func(*http.Request) (Principal, error)

// Authenticate implements RequestAuthenticator.
func (f RequestAuthenticatorFunc) Authenticate(r *http.Request) (Principal, error) { return f(r) }

// Failure returns a safe authentication failure with a semantic reason.
func Failure(reason string) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCodeUnauthorized, "Authentication required").WithReason(reason)
}
