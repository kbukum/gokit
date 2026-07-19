package connect

import (
	"context"
	"errors"
	"strings"
	"testing"

	connectrpc "connectrpc.com/connect"
	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	"github.com/kbukum/gokit/auth/jwt"
)

type authClaims struct {
	gojwt.RegisteredClaims
	UserID string `json:"user_id"`
}

func TestTokenAuthInterceptorRequiresBearerToken(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "wrong scheme", header: "Basic abc"},
		{name: "invalid token", header: "Bearer bad"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			validator := auth.TokenValidatorFunc(func(token string) (any, error) {
				if token != "good" {
					return nil, errors.New("invalid")
				}
				return authClaims{UserID: "user-1"}, nil
			})
			nextCalled := false
			req := connectrpc.NewRequest(&authClaims{})
			if tc.header != "" {
				req.Header().Set("Authorization", tc.header)
			}

			_, err := TokenAuthInterceptor(validator)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
				nextCalled = true
				return connectrpc.NewResponse(&authClaims{}), nil
			})(context.Background(), req)

			if err == nil {
				t.Fatal("expected authentication error")
			}
			if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
				t.Fatalf("CodeOf(err) = %v, want unauthenticated", connectrpc.CodeOf(err))
			}
			if nextCalled {
				t.Fatal("next handler should not run for failed authentication")
			}
		})
	}
}

func TestTokenAuthInterceptorStoresClaims(t *testing.T) {
	validator := auth.TokenValidatorFunc(func(token string) (any, error) {
		if token != "good" {
			return nil, errors.New("invalid")
		}
		return authClaims{UserID: "user-1"}, nil
	})
	req := connectrpc.NewRequest(&authClaims{})
	req.Header().Set("Authorization", "Bearer good")

	resp, err := TokenAuthInterceptor(validator)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		claims, ok := GetAuth[authClaims](ctx)
		if !ok {
			t.Fatal("claims missing from context")
		}
		if claims.UserID != "user-1" {
			t.Fatalf("UserID = %q, want user-1", claims.UserID)
		}
		return connectrpc.NewResponse(&authClaims{UserID: claims.UserID}), nil
	})(context.Background(), req)
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
	if got := resp.Any().(*authClaims).UserID; got != "user-1" {
		t.Fatalf("response UserID = %q, want user-1", got)
	}
}

func TestOptionalTokenAuthInterceptorContinuesWithoutValidToken(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "wrong scheme", header: "Basic abc"},
		{name: "invalid", header: "Bearer bad"},
	}
	validator := auth.TokenValidatorFunc(func(token string) (any, error) {
		if token != "good" {
			return nil, errors.New("invalid")
		}
		return authClaims{UserID: "user-1"}, nil
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := connectrpc.NewRequest(&authClaims{})
			if tc.header != "" {
				req.Header().Set("Authorization", tc.header)
			}

			_, err := OptionalTokenAuthInterceptor(validator)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
				if _, ok := GetAuth[authClaims](ctx); ok {
					t.Fatal("claims should not be present")
				}
				return connectrpc.NewResponse(&authClaims{}), nil
			})(context.Background(), req)
			if err != nil {
				t.Fatalf("interceptor returned error: %v", err)
			}
		})
	}
}

func TestOptionalTokenAuthInterceptorStoresValidClaims(t *testing.T) {
	validator := auth.TokenValidatorFunc(func(token string) (any, error) {
		return authClaims{UserID: token}, nil
	})
	req := connectrpc.NewRequest(&authClaims{})
	req.Header().Set("Authorization", "Bearer user-2")

	_, err := OptionalTokenAuthInterceptor(validator)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		claims, ok := GetAuth[authClaims](ctx)
		if !ok {
			t.Fatal("claims missing from context")
		}
		if claims.UserID != "user-2" {
			t.Fatalf("UserID = %q, want user-2", claims.UserID)
		}
		return connectrpc.NewResponse(&authClaims{}), nil
	})(context.Background(), req)
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestRequireAuthAndGetAuth(t *testing.T) {
	if _, err := RequireAuth[authClaims](context.Background()); err == nil {
		t.Fatal("RequireAuth should fail when context has no claims")
	} else if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
		t.Fatalf("CodeOf(err) = %v, want unauthenticated", connectrpc.CodeOf(err))
	}

	ctx := context.Background()
	claims := authClaims{UserID: "user-3"}
	ctx = authctx.Set(ctx, claims)
	got, err := RequireAuth[authClaims](ctx)
	if err != nil {
		t.Fatalf("RequireAuth returned error: %v", err)
	}
	if got.UserID != claims.UserID {
		t.Fatalf("UserID = %q, want %q", got.UserID, claims.UserID)
	}
	optional, ok := GetAuth[authClaims](ctx)
	if !ok || optional.UserID != claims.UserID {
		t.Fatalf("GetAuth() = %+v, %v; want claims", optional, ok)
	}
}

func TestJWTAuthInterceptors(t *testing.T) {
	jwtSvc := newJWTService(t)
	validToken, err := jwtSvc.GenerateAccess(&authClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:   "issuer",
			Audience: []string{"audience"},
		},
		UserID: "jwt-user",
	})
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}

	t.Run("required stores claims", func(t *testing.T) {
		req := connectrpc.NewRequest(&authClaims{})
		req.Header().Set("Authorization", "Bearer "+validToken)

		_, err := JWTAuthInterceptor(jwtSvc)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			claims, ok := GetAuth[*authClaims](ctx)
			if !ok {
				t.Fatal("claims missing from context")
			}
			if claims.UserID != "jwt-user" {
				t.Fatalf("UserID = %q, want jwt-user", claims.UserID)
			}
			return connectrpc.NewResponse(&authClaims{}), nil
		})(context.Background(), req)
		if err != nil {
			t.Fatalf("JWTAuthInterceptor returned error: %v", err)
		}
	})

	t.Run("required rejects invalid requests", func(t *testing.T) {
		for _, header := range []string{"", "Basic abc", "Bearer bad"} {
			req := connectrpc.NewRequest(&authClaims{})
			if header != "" {
				req.Header().Set("Authorization", header)
			}
			_, err := JWTAuthInterceptor(jwtSvc)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
				t.Fatal("next should not be called")
				return connectrpc.NewResponse(&authClaims{}), nil
			})(context.Background(), req)
			if err == nil || connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
				t.Fatalf("header %q: err code = %v, want unauthenticated", header, connectrpc.CodeOf(err))
			}
		}
	})

	t.Run("optional continues or stores claims", func(t *testing.T) {
		for _, header := range []string{"", "Basic abc", "Bearer bad"} {
			req := connectrpc.NewRequest(&authClaims{})
			if header != "" {
				req.Header().Set("Authorization", header)
			}
			_, err := OptionalJWTAuthInterceptor(jwtSvc)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
				if _, ok := GetAuth[*authClaims](ctx); ok {
					t.Fatal("claims should not be present")
				}
				return connectrpc.NewResponse(&authClaims{}), nil
			})(context.Background(), req)
			if err != nil {
				t.Fatalf("header %q: %v", header, err)
			}
		}

		req := connectrpc.NewRequest(&authClaims{})
		req.Header().Set("Authorization", "Bearer "+validToken)
		_, err := OptionalJWTAuthInterceptor(jwtSvc)(func(ctx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			claims, ok := GetAuth[*authClaims](ctx)
			if !ok || claims.UserID != "jwt-user" {
				t.Fatalf("claims = %+v, %v; want jwt-user", claims, ok)
			}
			return connectrpc.NewResponse(&authClaims{}), nil
		})(context.Background(), req)
		if err != nil {
			t.Fatalf("valid optional JWT returned error: %v", err)
		}
	})
}

func FuzzTokenAuthInterceptorBearerParsing(f *testing.F) {
	for _, seed := range []string{"", "Bearer ok", "Basic nope", "Bearer ", strings.Repeat("a", 128)} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, header string) {
		validator := auth.TokenValidatorFunc(func(token string) (any, error) {
			if token != "ok" {
				return nil, errors.New("invalid")
			}
			return authClaims{UserID: "ok"}, nil
		})
		req := connectrpc.NewRequest(&authClaims{})
		if header != "" {
			req.Header().Set("Authorization", header)
		}

		_, err := TokenAuthInterceptor(validator)(func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			return connectrpc.NewResponse(&authClaims{}), nil
		})(context.Background(), req)
		if header == "Bearer ok" && err != nil {
			t.Fatalf("valid header returned error: %v", err)
		}
	})
}

func newJWTService(t *testing.T) *jwt.Service[*authClaims] {
	t.Helper()

	cfg := &jwt.Config{
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Secret:             strings.Repeat("s", 32),
		Issuer:             "issuer",
		Audience:           []string{"audience"},
	}
	svc, err := jwt.NewService(cfg, func() *authClaims { return &authClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}
