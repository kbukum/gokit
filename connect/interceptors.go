package connect

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	protovalidate "buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/validation"
)

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// LoggingInterceptor returns a Connect interceptor that logs every RPC call with procedure name,
// duration, and outcome.
func LoggingInterceptor(log *logging.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			procedure := req.Spec().Procedure

			log.WithContext(ctx).DebugCtx(ctx, "RPC call started", map[string]any{
				"procedure": procedure,
			})

			resp, err := next(ctx, req)
			duration := time.Since(start)

			fields := map[string]any{
				"procedure":   procedure,
				"duration_ms": duration.Milliseconds(),
			}

			if err != nil {
				var connectErr *connect.Error
				if stderrors.As(err, &connectErr) {
					fields["code"] = connectErr.Code().String()
				}
				fields["error"] = err.Error()
				log.WithContext(ctx).ErrorCtx(ctx, "RPC call failed", fields)
			} else {
				fields["code"] = "ok"
				log.WithContext(ctx).DebugCtx(ctx, "RPC call completed", fields)
			}

			return resp, err
		}
	}
}

// ---------------------------------------------------------------------------
// Normalization  (handler error → coded Connect error)
// ---------------------------------------------------------------------------

// NormalizingInterceptor returns the one interceptor that converts every error a
// handler returns into a coded Connect error, for both unary and streaming RPCs.
// A handler that already returns a *connect.Error (for example from the auth or
// validation interceptor) is left untouched; anything else goes through
// errors.Normalize so context cancellation, deadlines, AppErrors, and unknown
// errors all map the same way and an unknown error's text never reaches the wire.
// The underlying cause is logged with the trace id and never serialized.
func NormalizingInterceptor(log *logging.Logger) connect.Interceptor {
	return &normalizingInterceptor{log: log}
}

type normalizingInterceptor struct {
	log *logging.Logger
}

func (i *normalizingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		return resp, i.normalize(ctx, req.Spec().Procedure, err)
	}
}

func (i *normalizingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *normalizingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return i.normalize(ctx, conn.Spec().Procedure, next(ctx, conn))
	}
}

func (i *normalizingInterceptor) normalize(ctx context.Context, procedure string, err error) error {
	if err == nil {
		return nil
	}

	// An *AppError anywhere in the chain owns its own encoding, even when it
	// wraps a Connect error as its cause, so it is never mistaken for an
	// already-coded passthrough. Only a non-AppError Connect error (from an auth
	// or validation interceptor) is left alone.
	// Only an explicitly returned native error is already approved for this boundary.
	if _, ok := err.(*connect.Error); ok { //nolint:errorlint // Unwrapping here would trust decoded remote failures.
		return err
	}

	appErr := apperrors.Normalize(err)
	i.logCause(ctx, procedure, appErr)
	encoded, encodeErr := ToConnectError(appErr, serviceDomain(procedure))
	if encodeErr != nil {
		safe := apperrors.Internal(encodeErr)
		i.logCause(ctx, procedure, safe)
		return connect.NewError(connect.CodeInternal, stderrors.New(safe.Message))
	}
	return encoded
}

// logCause records the underlying cause so an operator can trace an internal
// failure even though the cause never reaches the client.
func (i *normalizingInterceptor) logCause(ctx context.Context, procedure string, appErr *apperrors.AppError) {
	if i.log == nil || appErr == nil || appErr.Cause == nil {
		return
	}
	fields := map[string]any{
		"procedure": procedure,
		"code":      string(appErr.Code),
		"cause":     appErr.Cause.Error(),
	}
	if appErr.TraceID != "" {
		fields["traceId"] = appErr.TraceID
	}
	i.log.WithContext(ctx).ErrorCtx(ctx, "RPC handler error normalized", fields)
}

// ---------------------------------------------------------------------------
// Validation  (protovalidate → shared violations)
// ---------------------------------------------------------------------------

// ValidationInterceptor returns a unary interceptor that validates each request
// message with the injected protovalidate.Validator and reports failures as the
// shared violation vocabulary with canonical field paths. The validator is
// injected, never a package-level singleton. A request that is not a proto
// message is passed through untouched.
func ValidationInterceptor(v protovalidate.Validator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			msg, ok := req.Any().(proto.Message)
			if !ok {
				return next(ctx, req)
			}
			if err := v.Validate(msg); err != nil {
				if violations, matched := violationsFromProtovalidate(err); matched {
					appErr := apperrors.Validation("request failed validation").WithViolations(violations...)
					return nil, appErr
				}
				return nil, apperrors.Internal(err)
			}
			return next(ctx, req)
		}
	}
}

// violationsFromProtovalidate converts a protovalidate failure into the shared
// violation shape, rendering the canonical dotted field path for each violation.
func violationsFromProtovalidate(err error) ([]apperrors.Violation, bool) {
	var valErr *protovalidate.ValidationError
	if !stderrors.As(err, &valErr) {
		return nil, false
	}
	violations := make([]apperrors.Violation, 0, len(valErr.Violations))
	for _, v := range valErr.Violations {
		p := v.Proto
		violations = append(violations, apperrors.Violation{
			Field:   fieldPath(p),
			Reason:  validation.ReasonForRule(p.GetRuleId()),
			Message: p.GetMessage(),
		})
	}
	return violations, true
}

func fieldPath(v *validate.Violation) string {
	return protovalidate.FieldPathString(v.GetField())
}

// ---------------------------------------------------------------------------
// Server deadline for unary RPCs
// ---------------------------------------------------------------------------

// DeadlineInterceptor returns a unary interceptor that bounds how long a unary
// RPC may run. It clamps a client deadline that is further out than max down to
// max, and applies max when the client sends no deadline at all. Dependencies must honor cancellation; this does not forcibly terminate handlers. A client deadline that is
// already tighter than max is left in place. A non-positive max disables the
// bound. Streams get their own idle and lifetime rules elsewhere.
func DeadlineInterceptor(maxDuration time.Duration) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if maxDuration <= 0 {
				return next(ctx, req)
			}
			bound := time.Now().Add(maxDuration)
			if existing, ok := ctx.Deadline(); ok && !existing.After(bound) {
				return next(ctx, req)
			}
			ctx, cancel := context.WithDeadline(ctx, bound)
			defer cancel()
			return next(ctx, req)
		}
	}
}

// serviceDomain extracts the "package.Service" grouping from a Connect procedure
// of the form "/package.Service/Method" for use as the ErrorInfo domain.
func serviceDomain(procedure string) string {
	trimmed := strings.TrimPrefix(procedure, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}
