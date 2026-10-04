package auth

import (
	"net/http"
	"strings"

	"github.com/kbukum/gokit/util"
)

// SessionCookie is the secure host-only browser credential name.
const SessionCookie = "__Host-session"

// PresentedCredentials is the parsed single credential. Its contents must never be logged.
type PresentedCredentials struct {
	Kind  Credential
	Value string
}

// ParseCredentials rejects duplicate, empty, mixed and reserved unsupported credentials.
func ParseCredentials(r *http.Request) (PresentedCredentials, error) {
	var out PresentedCredentials
	for name := range r.Header {
		if strings.EqualFold(name, "Authorization") {
			return out, Failure("UNSUPPORTED_CREDENTIAL")
		}
	}
	var keys []string
	for name, values := range r.Header {
		if strings.EqualFold(name, "X-API-Key") {
			keys = append(keys, values...)
		}
	}
	var cookies []string
	for _, line := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(line, ";") {
			name, value, found := strings.Cut(strings.TrimSpace(part), "=")
			if name == SessionCookie {
				if !found {
					return out, Failure("INVALID_CREDENTIAL")
				}
				cookies = append(cookies, value)
			}
		}
	}
	if len(keys) > 1 || len(cookies) > 1 || (len(keys) != 0 && len(cookies) != 0) {
		return out, Failure("AMBIGUOUS_CREDENTIAL")
	}
	if len(keys) == 1 {
		if keys[0] == "" || len(keys[0]) > 512 || strings.TrimSpace(keys[0]) != keys[0] {
			return out, Failure("INVALID_CREDENTIAL")
		}
		return PresentedCredentials{Kind: APIKey, Value: keys[0]}, nil
	}
	if len(cookies) == 1 {
		if cookies[0] == "" {
			return out, Failure("INVALID_CREDENTIAL")
		}
		return PresentedCredentials{Kind: Session, Value: cookies[0]}, nil
	}
	return out, nil
}

// Chain authenticates the complete request at the transport boundary. present is false only when the request carries no credential at all; the caller's missing-credential policy decides whether that is anonymous or rejected. A false present never comes with an identity.
type Chain interface {
	Authenticate(*http.Request) (p Principal, present bool, err error)
}

type chain struct {
	session, key RequestAuthenticator
}

// NewChain dispatches exactly one session cookie or API key to its validator.
func NewChain(session, key RequestAuthenticator) Chain {
	return &chain{session: session, key: key}
}

func (c *chain) Authenticate(r *http.Request) (Principal, bool, error) {
	credential, err := ParseCredentials(r)
	if err != nil {
		return Principal{}, true, err
	}
	var validator RequestAuthenticator
	switch credential.Kind {
	case Session:
		validator = c.session
	case APIKey:
		validator = c.key
	default:
		return Principal{}, false, nil
	}
	if util.IsNil(validator) {
		return Principal{}, true, Failure("UNSUPPORTED_CREDENTIAL")
	}
	p, err := validator.Authenticate(r)
	if err != nil {
		return Principal{}, true, err
	}
	if err := p.Validate(); err != nil {
		return Principal{}, true, err
	}
	return p.Clone(), true, nil
}
