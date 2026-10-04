package connect

import (
	"context"

	"connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// ClaimsGetter retrieves a typed identity established by the outer HTTP authentication middleware. Inject the authentication provider's getter; Connect never parses or validates credentials.
type ClaimsGetter[T any] func(context.Context) (T, bool)

// AuthInterceptor requires an identity already verified by the outer HTTP middleware. Cookie authentication and CSRF must run there with the original HTTP method and duplicate headers intact.
func AuthInterceptor[T any](getClaims ClaimsGetter[T]) (connect.UnaryInterceptorFunc, error) {
	if getClaims == nil {
		return nil, apperrors.InvalidInput("getClaims", "a non-nil ClaimsGetter is required")
	}
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if _, err := RequireAuth(ctx, getClaims); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}, nil
}

// RequireAuth returns the verified identity or an unauthenticated transport error. Optional handlers can call the injected getter directly after the outer middleware has rejected invalid presented credentials.
func RequireAuth[T any](ctx context.Context, getClaims ClaimsGetter[T]) (T, error) {
	var zero T
	if getClaims != nil {
		claims, ok := getClaims(ctx)
		if ok && !util.IsNil(claims) {
			return claims, nil
		}
	}
	return zero, (&normalizingInterceptor{}).normalize(ctx, "", apperrors.Unauthorized(""))
}
