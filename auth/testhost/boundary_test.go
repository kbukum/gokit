package testhost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	kitconnect "github.com/kbukum/gokit/connect"
	"github.com/kbukum/gokit/server/middleware"
)

func TestOptionalBoundaryKeepsRequiredGuardClosed(t *testing.T) {
	t.Parallel()
	key := auth.RequestAuthenticatorFunc(func(r *http.Request) (auth.Principal, error) {
		if r.Header.Get("X-API-Key") != "key.valid" {
			return auth.Principal{}, auth.Failure("INVALID_CREDENTIAL")
		}
		return auth.Principal{Subject: "svc", Kind: auth.Service, Credential: auth.APIKey, Reference: "ref", ExpiresAt: time.Now().Add(time.Hour), Restrictions: auth.Restrictions{Mode: auth.Unrestricted}}, nil
	})
	optional, err := middleware.HTTPAuth(auth.NewChain(nil, key), authctx.Set[auth.Principal], middleware.WithMissingPolicy(middleware.AcceptMissing))
	if err != nil {
		t.Fatal(err)
	}
	require, err := kitconnect.AuthInterceptor(authctx.Get[auth.Principal])
	if err != nil {
		t.Fatal(err)
	}
	path := "/fixture.v1.Service/Call"
	handler := connect.NewUnaryHandler(path, func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
		return connect.NewResponse(&emptypb.Empty{}), nil
	}, connect.WithInterceptors(require))
	server := httptest.NewServer(optional(handler))
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name string
		key  string
		want int
	}{
		{name: "missing credentials stay anonymous and fail the required guard", want: http.StatusUnauthorized},
		{name: "invalid credentials fail at the outer boundary", key: "key.invalid", want: http.StatusUnauthorized},
		{name: "valid credentials pass", key: "key.valid", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.key != "" {
				req.Header.Set("X-API-Key", tc.key)
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
