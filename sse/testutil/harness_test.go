package testutil_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/apipb"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func testConfig(auth sse.Authorizer) sse.HandlerConfig {
	cfg := sse.DefaultHandlerConfig()
	cfg.Authorize, cfg.Logger = auth, logging.NewDefault("test")
	return cfg
}

func TestAuthenticatedScopedReplayAndConcurrentStreams(t *testing.T) {
	t.Parallel()
	auth := testutil.AllowAuthenticator("alice")
	h := testutil.New(t, sse.DefaultLimits(), testConfig(sse.Authenticated(auth,
		func(r *http.Request, id string) (sse.Access, error) {
			seen, ok := sse.IdentityFromContext[string](r.Context())
			if !ok || seen != id {
				return sse.Access{}, apperrors.Unauthorized("")
			}
			return sse.Access{Principal: "alice", Route: "alice"}, nil
		})))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	start := h.Bus.Cursor()
	if err := h.Bus.Publish(ctx, "bob", &apipb.Method{Name: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Bus.Publish(ctx, "alice", &apipb.Method{Name: "visible"}); err != nil {
		t.Fatal(err)
	}
	first, err := h.Resume(ctx, "header-token", start)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := h.Resume(ctx, "header-token", start)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, stream := range []*testutil.StreamClient{first, second} {
		testutil.RequireStatus(t, stream, 200)
		connected := stream.SkipConnected(t)
		if !strings.Contains(string(connected.Data), `"epoch"`) || strings.Contains(string(connected.Data), "alice") {
			t.Fatalf("handshake: %s", connected.Data)
		}
		var payload struct {
			Name string `json:"name"`
		}
		ev := stream.RequireJSON(t, "google.protobuf.Method", &payload)
		if payload.Name != "visible" || !strings.HasSuffix(ev.ID, ":2") {
			t.Fatalf("scoped replay: %+v", ev)
		}
	}
	if auth.Calls() != 2 {
		t.Fatal("authentication was bypassed")
	}
}

func TestAuthenticationFailures(t *testing.T) {
	t.Parallel()
	resolve := func(*http.Request, string) (sse.Access, error) {
		return sse.Access{Principal: "a", Route: "a"}, nil
	}
	var typedNil sse.AuthenticatorFunc[string]
	for _, tc := range []struct {
		name   string
		auth   sse.Authorizer
		status int
	}{
		{"missing", sse.Authenticated(sse.BearerAuthenticator[string](nil), resolve), 401},
		{"unauthorized", sse.Authenticated(testutil.RejectUnauthorized[string]("safe"), resolve), 401},
		{"forbidden", sse.Authenticated(testutil.RejectForbidden[string]("safe"), resolve), 403},
		{"nil", sse.Authenticated(nil, resolve), 401},
		{"typed nil", sse.Authenticated(typedNil, resolve), 401},
		{"nil resolver", sse.Authenticated(testutil.AllowAuthenticator("a"), nil), 401},
		{"resolver forbidden", sse.Authenticated(testutil.AllowAuthenticator("a"), func(*http.Request, string) (sse.Access, error) {
			return sse.Access{}, apperrors.Forbidden("")
		}), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := testutil.New(t, sse.DefaultLimits(), testConfig(tc.auth))
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			stream := h.MustConnect(t, ctx, "")
			testutil.RequireStatus(t, stream, tc.status)
			if h.Bus.Stats().AllocatedQueues != 0 {
				t.Fatal("rejection allocated a queue")
			}
		})
	}
}
