package testutil_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestAuthenticatedOpen_PerPrincipalScoping(t *testing.T) {
	t.Parallel()

	auth := testutil.AllowAuthenticator("alice")
	var resolvedIdentity any
	h := testutil.New(t, "base",
		sse.WithAuthenticator(auth),
		sse.WithClientIdentity(func(r *http.Request, identity any) (string, []sse.ClientOption, error) {
			resolvedIdentity, _ = sse.IdentityFromContext(r.Context())
			return "user:" + identity.(string), []sse.ClientOption{sse.WithUserID(identity.(string))}, nil
		}),
	)

	stream := h.MustConnect(t, testContext(t), "token")
	defer stream.Close()
	testutil.RequireStatus(t, stream, http.StatusOK)

	connected := stream.SkipConnected(t)
	if want := `"user_id":"alice"`; !strings.Contains(string(connected.Data), want) {
		t.Fatalf("expected %s in connected event, got %q", want, connected.Data)
	}
	if resolvedIdentity != "alice" {
		t.Fatalf("resolver should see identity via context, got %v", resolvedIdentity)
	}

	// Registration is synchronous (ServeSSE registers the client before writing
	// the connected handshake, and Hub.Register blocks until the client is in the
	// map), so SkipConnected above already guarantees the per-principal routing
	// key is registered — no polling needed before broadcasting.
	h.Hub.BroadcastFrame("user:alice", sse.Frame{Event: "ping", Data: []byte(`{"n":1}`)})

	var payload struct {
		N int `json:"n"`
	}
	stream.RequireJSON(t, "ping", &payload)
	if payload.N != 1 {
		t.Fatalf("expected n=1, got %d", payload.N)
	}

	// A different principal's key must not reach this stream.
	h.Hub.BroadcastFrame("user:bob", sse.Frame{Event: "ping", Data: []byte(`{"n":2}`)})
	h.Hub.BroadcastFrame("user:alice", sse.Frame{Event: "done", Data: []byte(`{}`)})
	if evt := stream.Require(t, "done"); evt.Name != "done" {
		t.Fatalf("expected to skip bob's event and receive done, got %q", evt.Name)
	}

	if auth.Calls() != 1 {
		t.Fatalf("expected authenticator to run once, ran %d", auth.Calls())
	}
}

func TestMissingCredentialRejected(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(sse.BearerAuthenticator(stubValidator{})),
	)
	stream := h.MustConnect(t, testContext(t), "") // no Authorization header
	testutil.RequireStatus(t, stream, http.StatusUnauthorized)
}

func TestInvalidCredentialRejected(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(sse.BearerAuthenticator(stubValidator{err: apperrors.InvalidToken()})),
	)
	stream := h.MustConnect(t, testContext(t), "bad-token")
	testutil.RequireStatus(t, stream, http.StatusUnauthorized)
}

func TestForbiddenPrincipalRejected(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(testutil.RejectForbidden("not allowed")),
	)
	stream := h.MustConnect(t, testContext(t), "token")
	testutil.RequireStatus(t, stream, http.StatusForbidden)
}

func TestResolverForbiddenRejected(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(testutil.AllowAuthenticator("alice")),
		sse.WithClientIdentity(func(*http.Request, any) (string, []sse.ClientOption, error) {
			return "", nil, apperrors.Forbidden("stream not permitted")
		}),
	)
	stream := h.MustConnect(t, testContext(t), "token")
	testutil.RequireStatus(t, stream, http.StatusForbidden)
}

func TestUnauthenticatedEndpointStillWorks(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "open:1") // no authenticator
	stream := h.MustConnect(t, testContext(t), "")
	defer stream.Close()
	testutil.RequireStatus(t, stream, http.StatusOK)
	stream.SkipConnected(t)

	// SkipConnected guarantees the client is registered (synchronous Register),
	// so the broadcast below cannot race registration.
	h.Hub.BroadcastFrame("open:1*", sse.Frame{Event: "msg", Data: []byte(`{}`)})
	stream.Require(t, "msg")
}

// TestResolverResolvedRouteWins verifies the resolved routing key wins even when
// the resolver also returns a WithRoute in its options: the authoritative route
// is applied last so per-principal scoping cannot be overridden.
func TestResolverResolvedRouteWins(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(testutil.AllowAuthenticator("alice")),
		sse.WithClientIdentity(func(_ *http.Request, id any) (string, []sse.ClientOption, error) {
			// Return a decoy route in the options; the resolved id must still win.
			return "user:" + id.(string), []sse.ClientOption{sse.WithRoute("decoy")}, nil
		}),
	)

	stream := h.MustConnect(t, testContext(t), "token")
	defer stream.Close()
	testutil.RequireStatus(t, stream, http.StatusOK)
	stream.SkipConnected(t)

	// The decoy route must not receive; the authoritative per-principal route must.
	h.Hub.BroadcastFrame("decoy", sse.Frame{Event: "wrong", Data: []byte(`{}`)})
	h.Hub.BroadcastFrame("user:alice", sse.Frame{Event: "right", Data: []byte(`{}`)})
	if evt := stream.Require(t, "right"); evt.Name != "right" {
		t.Fatalf("expected authoritative route to win, got %q", evt.Name)
	}
}

// stubValidator is a no-op TokenValidator for header-path tests.
type stubValidator struct{ err error }

func (s stubValidator) ValidateToken(string) (any, error) { return "claims", s.err }

// TestConcurrentStreamsSamePrincipal covers two simultaneous streams resolving to
// the same routing key: each keeps a unique registration id, so neither evicts
// the other and a broadcast to the shared route reaches both.
func TestConcurrentStreamsSamePrincipal(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(testutil.AllowAuthenticator("alice")),
		sse.WithClientIdentity(func(_ *http.Request, id any) (string, []sse.ClientOption, error) {
			return "user:" + id.(string), nil, nil
		}),
	)

	first := h.MustConnect(t, testContext(t), "token-1")
	defer first.Close()
	testutil.RequireStatus(t, first, http.StatusOK)
	first.SkipConnected(t)

	second := h.MustConnect(t, testContext(t), "token-2")
	defer second.Close()
	testutil.RequireStatus(t, second, http.StatusOK)
	second.SkipConnected(t)

	// Both SkipConnected calls above guarantee both connections are registered
	// under the shared route (synchronous Register), so a single broadcast reaches
	// both without polling.
	h.Hub.BroadcastFrame("user:alice", sse.Frame{Event: "ping", Data: []byte(`{}`)})
	first.Require(t, "ping")
	second.Require(t, "ping")
}

// TestNilAuthenticatorFailsClosed verifies WithAuthenticator(nil) installs a
// fail-closed gate rather than leaving the endpoint publicly accessible.
func TestNilAuthenticatorFailsClosed(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base", sse.WithAuthenticator(nil))
	stream := h.MustConnect(t, testContext(t), "token")
	testutil.RequireStatus(t, stream, http.StatusUnauthorized)
}

// TestNilIdentityRejected verifies an authenticator that returns (nil, nil) is
// treated as a rejection rather than admitting an identity-less connection.
func TestNilIdentityRejected(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithAuthenticator(sse.AuthenticatorFunc(func(*http.Request) (any, error) {
			return nil, nil //nolint:nilnil // exercising the (nil, nil) admit path the handler must reject
		})),
	)
	stream := h.MustConnect(t, testContext(t), "token")
	testutil.RequireStatus(t, stream, http.StatusUnauthorized)
}

// TestResolverWithoutAuthenticatorFailsClosed verifies configuring an identity
// resolver without an authenticator fails closed instead of serving the intended
// per-principal endpoint unauthenticated.
func TestResolverWithoutAuthenticatorFailsClosed(t *testing.T) {
	t.Parallel()

	h := testutil.New(t, "base",
		sse.WithClientIdentity(func(*http.Request, any) (string, []sse.ClientOption, error) {
			return "user:x", nil, nil
		}),
	)
	stream := h.MustConnect(t, testContext(t), "token")
	testutil.RequireStatus(t, stream, http.StatusUnauthorized)
}
