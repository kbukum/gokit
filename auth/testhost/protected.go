package testhost

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kbukum/gokit/auth"
	"github.com/kbukum/gokit/auth/authctx"
	kitconnect "github.com/kbukum/gokit/connect"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server/middleware"
	"github.com/kbukum/gokit/sse"
)

func (h *Host) mountProtected(chain auth.Chain) error {
	errorWriter := connect.NewErrorWriter()
	writeFailure := func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != OperationPath || !errorWriter.IsSupported(r) {
			h.writeError(w, r, err)
			return
		}
		encoded, encodeErr := kitconnect.ToConnectError(apperrors.Normalize(err), "gokit.auth.v1")
		if encodeErr != nil {
			h.writeError(w, r, encodeErr)
			return
		}
		if err := errorWriter.Write(w, r, encoded); err != nil {
			h.log.ErrorCtx(r.Context(), "Fixture RPC failure write failed", map[string]any{"error": err.Error()})
		}
	}
	protect, err := middleware.HTTPAuth(chain, authctx.Set[auth.Principal], middleware.WithAuthErrorWriter(writeFailure))
	if err != nil {
		return err
	}
	require, err := kitconnect.AuthInterceptor(authctx.Get[auth.Principal])
	if err != nil {
		return err
	}
	operation := connect.NewUnaryHandler(OperationPath, func(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[wrapperspb.StringValue], error) {
		principal, err := kitconnect.RequireAuth(ctx, authctx.Get[auth.Principal])
		if err != nil {
			return nil, err
		}
		if err := auth.Authorize(ctx, fixturePolicy{}, principal, "resource-a", "read"); err != nil {
			return nil, err
		}
		return connect.NewResponse(wrapperspb.String(string(principal.Kind) + ":" + principal.Subject)), nil
	}, connect.WithInterceptors(kitconnect.NormalizingInterceptor(h.log), require))
	h.server.Handle(OperationPath, h.track(protect(operation)))
	h.server.Handle(EventsPath, h.track(protect(http.HandlerFunc(h.stream))))
	return nil
}

type fixturePolicy struct{}

func (fixturePolicy) Authorize(ctx context.Context, p auth.Principal, resource string, _ []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Subject != "fixture-user" || (resource != "resource-a" && resource != "resource-b") {
		return apperrors.New(apperrors.ErrCodeForbidden, "Permission denied")
	}
	return nil
}

func (h *Host) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, ok := authctx.Get[auth.Principal](ctx)
	if !ok {
		h.writeError(w, r, auth.Failure("MISSING_CREDENTIAL"))
		return
	}
	lifetime := ctx
	if p.Credential == auth.Session {
		owned, release, err := h.manager.Acquire(ctx, p.Reference)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		defer release()
		lifetime = owned
	}
	cfg := sse.DefaultHandlerConfig()
	cfg.Logger = h.log
	cfg.Authorize = func(*http.Request) (sse.Access, error) {
		if err := auth.Authorize(ctx, fixturePolicy{}, p, "resource-a", "read"); err != nil {
			return sse.Access{}, err
		}
		return sse.Access{Principal: p.Subject, Route: "fixture", Lifetime: lifetime}, nil
	}
	handler, err := sse.NewHandler(h.bus, cfg)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.ServeHTTP(w, r)
}
