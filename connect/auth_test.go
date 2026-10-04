package connect

import (
	"context"
	"errors"
	"testing"

	connectrpc "connectrpc.com/connect"
)

type (
	authClaims    struct{ UserID string }
	authClaimsKey struct{}
)

func getTestClaims(ctx context.Context) (authClaims, bool) {
	claims, ok := ctx.Value(authClaimsKey{}).(authClaims)
	return claims, ok
}

func TestAuthInterceptorRequiresVerifiedContext(t *testing.T) {
	t.Parallel()
	interceptor, err := AuthInterceptor(getTestClaims)
	if err != nil {
		t.Fatal(err)
	}
	req := connectrpc.NewRequest(&authClaims{})
	req.Header().Set("Authorization", "Bearer "+"unverified")
	req.Header().Set("Cookie", "session=unverified")
	_, err = interceptor(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		t.Fatal("unverified credential admitted")
		return connectrpc.NewResponse(&authClaims{}), nil
	})(t.Context(), req)
	if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
		t.Fatalf("code=%v", connectrpc.CodeOf(err))
	}
}

func TestAuthInterceptorPreservesVerifiedContextAndResult(t *testing.T) {
	t.Parallel()
	interceptor, err := AuthInterceptor(getTestClaims)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), authClaimsKey{}, authClaims{UserID: "alice"}))
	cancel()
	want := errors.New("downstream")
	response := connectrpc.NewResponse(&authClaims{UserID: "alice"})
	got, err := interceptor(func(got context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		claims, authErr := RequireAuth(got, getTestClaims)
		if authErr != nil || claims.UserID != "alice" || got != ctx || got.Err() != context.Canceled {
			t.Fatal("verified claims or request context lost")
		}
		return response, want
	})(ctx, connectrpc.NewRequest(&authClaims{}))
	if got != response || !errors.Is(err, want) {
		t.Fatalf("response=%v error=%v", got, err)
	}
}

func TestRequireAuthAndNilWiring(t *testing.T) {
	t.Parallel()
	if _, err := AuthInterceptor[authClaims](nil); err == nil {
		t.Fatal("accepted nil accessor")
	}
	if _, err := RequireAuth[authClaims](t.Context(), nil); connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
		t.Fatalf("nil accessor: %v", err)
	}
	if _, err := RequireAuth(t.Context(), getTestClaims); connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
		t.Fatalf("missing claims: %v", err)
	}
	getNil := func(context.Context) (*authClaims, bool) { return nil, true }
	if _, err := RequireAuth(t.Context(), getNil); connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
		t.Fatalf("typed nil claims: %v", err)
	}
}
