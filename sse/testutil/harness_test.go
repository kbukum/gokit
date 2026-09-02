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

	// Broadcast to the per-principal routing key derived from the identity.
	waitForClient(t, h, "user:alice")
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

	waitForClient(t, h, "open:1")
	h.Hub.BroadcastFrame("open:1", sse.Frame{Event: "msg", Data: []byte(`{}`)})
	stream.Require(t, "msg")
}

// stubValidator is a no-op TokenValidator for header-path tests.
type stubValidator struct{ err error }

func (s stubValidator) ValidateToken(string) (any, error) { return "claims", s.err }

// waitForClient blocks until the hub has registered clientID, so a broadcast is
// not raced against asynchronous registration.
func waitForClient(t *testing.T, h *testutil.Harness, clientID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.Hub.GetClient(clientID) != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("client %q never registered", clientID)
}
