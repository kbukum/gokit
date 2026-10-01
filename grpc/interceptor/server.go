package interceptor

import (
	"context"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/logging"
)

// UnaryServerLoggingInterceptor returns a unary server interceptor that logs each incoming RPC with method,
// duration, and status code.
func UnaryServerLoggingInterceptor(log *logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		svc := serviceDomain(info.FullMethod)
		method := path.Base(info.FullMethod)

		log.DebugCtx(ctx, "gRPC request started", map[string]any{
			"service": svc,
			"method":  method,
		})

		resp, err := handler(ctx, req)
		duration := time.Since(start)

		fields := map[string]any{
			"service":     svc,
			"method":      method,
			"duration_ms": duration.Milliseconds(),
		}

		if err != nil {
			st := status.Convert(err)
			fields["status"] = st.Code().String()
			fields["error"] = st.Message()
			log.ErrorCtx(ctx, "gRPC request failed", fields)
		} else {
			fields["status"] = "OK"
			log.DebugCtx(ctx, "gRPC request completed", fields)
		}

		return resp, err
	}
}

// UnaryServerNormalizingInterceptor returns the one unary interceptor that turns
// every error a handler returns into a coded gRPC status, mirroring the Connect
// normalizing interceptor. An error that is already a gRPC status (for example
// from a validation or auth interceptor) is left untouched; anything else goes
// through errors.Normalize and the one code table, so context cancellation,
// deadlines, AppErrors, and unknown errors all map the same way and an unknown
// error's text never reaches the wire. The underlying cause is logged with the
// trace id and never serialized.
func UnaryServerNormalizingInterceptor(log *logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		return resp, normalizeServerError(ctx, log, info.FullMethod, err)
	}
}

// StreamServerNormalizingInterceptor is the streaming counterpart of
// UnaryServerNormalizingInterceptor.
func StreamServerNormalizingInterceptor(log *logging.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, ss)
		if err == nil {
			return nil
		}
		return normalizeServerError(ss.Context(), log, info.FullMethod, err)
	}
}

// normalizeServerError leaves an already-coded status in place and otherwise
// maps the error through the one code table, logging the cause. An *AppError
// anywhere in the chain owns its own encoding even when it wraps a transport
// status as its cause, so it is never mistaken for an already-coded passthrough.
func normalizeServerError(ctx context.Context, log *logging.Logger, fullMethod string, err error) error {
	// Only an explicitly returned native error is already approved for this boundary.
	if _, ok := err.(interface{ GRPCStatus() *status.Status }); ok {
		return err
	}
	appErr := apperrors.Normalize(err)
	logCause(ctx, log, fullMethod, appErr)
	encoded, encodeErr := grpccfg.AppErrorToStatus(appErr, serviceDomain(fullMethod))
	if encodeErr != nil {
		safe := apperrors.Internal(encodeErr)
		logCause(ctx, log, fullMethod, safe)
		return status.Error(codes.Internal, safe.Message)
	}
	return encoded.Err()
}

// logCause records the underlying cause so an operator can trace an internal
// failure even though the cause never reaches the client.
func logCause(ctx context.Context, log *logging.Logger, fullMethod string, appErr *apperrors.AppError) {
	if log == nil || appErr == nil || appErr.Cause == nil {
		return
	}
	fields := map[string]any{
		"service": serviceDomain(fullMethod),
		"method":  path.Base(fullMethod),
		"code":    string(appErr.Code),
		"cause":   appErr.Cause.Error(),
	}
	if appErr.TraceID != "" {
		fields["traceId"] = appErr.TraceID
	}
	log.ErrorCtx(ctx, "gRPC handler error normalized", fields)
}

// UnaryServerDeadlineInterceptor returns a unary interceptor that bounds how long
// a unary RPC may run. It clamps a client deadline that is further out than max
// down to max, and applies max when the client sends no deadline at all. Dependencies must honor cancellation; this does not forcibly terminate handlers. A client deadline
// that is already tighter than max is left in place. A non-positive max disables
// the bound. Streams get their own idle and lifetime rules elsewhere.
func UnaryServerDeadlineInterceptor(maxDuration time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if maxDuration <= 0 {
			return handler(ctx, req)
		}
		bound := time.Now().Add(maxDuration)
		if existing, ok := ctx.Deadline(); ok && !existing.After(bound) {
			return handler(ctx, req)
		}
		ctx, cancel := context.WithDeadline(ctx, bound)
		defer cancel()
		return handler(ctx, req)
	}
}

// serviceDomain extracts the "package.Service" grouping from a gRPC full method
// of the form "/package.Service/Method" for use as the ErrorInfo domain and in
// log fields.
func serviceDomain(fullMethod string) string {
	dir := path.Dir(fullMethod)
	if dir != "" && dir[0] == '/' {
		return dir[1:]
	}
	return dir
}
