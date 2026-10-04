package connect

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
	"time"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	protovalidate "buf.build/go/protovalidate"
	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

func newProtoRequest() connectrpc.AnyRequest {
	return connectrpc.NewRequest(&emptypb.Empty{})
}

func unaryHandler(resp connectrpc.AnyResponse, err error) connectrpc.UnaryFunc {
	return func(context.Context, connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		return resp, err
	}
}

func okResp() connectrpc.AnyResponse { return connectrpc.NewResponse(&emptypb.Empty{}) }

// ---------------------------------------------------------------------------
// NormalizingInterceptor
// ---------------------------------------------------------------------------

func TestNormalizingInterceptor_Unary(t *testing.T) {
	t.Parallel()
	interceptor := NormalizingInterceptor(nil)

	t.Run("success passes through", func(t *testing.T) {
		t.Parallel()
		want := okResp()
		resp, err := interceptor.WrapUnary(unaryHandler(want, nil))(context.Background(), newProtoRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != want {
			t.Fatal("response not passed through")
		}
	})

	t.Run("existing connect error passes through", func(t *testing.T) {
		t.Parallel()
		want := connectrpc.NewError(connectrpc.CodePermissionDenied, stderrors.New("denied"))
		_, err := interceptor.WrapUnary(unaryHandler(nil, want))(context.Background(), newProtoRequest())
		if !stderrors.Is(err, want) {
			t.Fatalf("error = %v, want original connect error", err)
		}
	})

	t.Run("app error maps to its code", func(t *testing.T) {
		t.Parallel()
		_, err := interceptor.WrapUnary(unaryHandler(nil, apperrors.Unauthorized("login required")))(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
			t.Fatalf("code = %v, want unauthenticated", connectrpc.CodeOf(err))
		}
	})

	t.Run("wrapped cancellation maps to canceled", func(t *testing.T) {
		t.Parallel()
		wrapped := fmt.Errorf("handler: %w", context.Canceled)
		_, err := interceptor.WrapUnary(unaryHandler(nil, wrapped))(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeCanceled {
			t.Fatalf("code = %v, want canceled", connectrpc.CodeOf(err))
		}
	})

	t.Run("wrapped deadline maps to deadline_exceeded", func(t *testing.T) {
		t.Parallel()
		wrapped := fmt.Errorf("handler: %w", context.DeadlineExceeded)
		_, err := interceptor.WrapUnary(unaryHandler(nil, wrapped))(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeDeadlineExceeded {
			t.Fatalf("code = %v, want deadline_exceeded", connectrpc.CodeOf(err))
		}
	})

	t.Run("unknown error is internal and never leaks text", func(t *testing.T) {
		t.Parallel()
		secret := "connection string postgres://user:pw@host/db"
		_, err := interceptor.WrapUnary(unaryHandler(nil, stderrors.New(secret)))(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeInternal {
			t.Fatalf("code = %v, want internal", connectrpc.CodeOf(err))
		}
		if strings.Contains(err.Error(), "postgres") {
			t.Fatalf("internal cause leaked to client: %q", err.Error())
		}
	})

	t.Run("app error wrapping a connect cause owns its own encoding", func(t *testing.T) {
		t.Parallel()
		leak := "private backend diagnostic"
		cause := connectrpc.NewError(connectrpc.CodeUnavailable, stderrors.New(leak))
		wrapped := apperrors.Internal(cause)
		_, err := interceptor.WrapUnary(unaryHandler(nil, wrapped))(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeInternal {
			t.Fatalf("code = %v, want internal", connectrpc.CodeOf(err))
		}
		if got := mustDecode(t, err); got.Code != apperrors.ErrCodeInternal {
			t.Fatalf("decoded code = %q, want %q", got.Code, apperrors.ErrCodeInternal)
		}
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("internal cause leaked to client: %q", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// ValidationInterceptor
// ---------------------------------------------------------------------------

type fakeValidator struct {
	err error
}

func (f fakeValidator) Validate(proto.Message, ...protovalidate.ValidationOption) error {
	return f.err
}

func newValidationError(field, rule, msg string) *protovalidate.ValidationError {
	element := &validate.FieldPathElement{}
	element.SetFieldName(field)
	path := &validate.FieldPath{}
	path.SetElements([]*validate.FieldPathElement{element})
	v := &validate.Violation{}
	v.SetField(path)
	v.SetRuleId(rule)
	v.SetMessage(msg)
	return &protovalidate.ValidationError{Violations: []*protovalidate.Violation{{Proto: v}}}
}

func TestValidationInterceptor(t *testing.T) {
	t.Parallel()

	t.Run("valid request proceeds", func(t *testing.T) {
		t.Parallel()
		called := false
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			called = true
			return okResp(), nil
		}
		_, err := ValidationInterceptor(fakeValidator{})(next)(context.Background(), newProtoRequest())
		if err != nil || !called {
			t.Fatalf("expected handler to run; called=%v err=%v", called, err)
		}
	})

	t.Run("violations become shared violations", func(t *testing.T) {
		t.Parallel()
		v := fakeValidator{err: newValidationError("email", "required", "is required")}
		_, err := ValidationInterceptor(v)(unaryHandler(okResp(), nil))(context.Background(), newProtoRequest())
		appErr, ok := apperrors.AsAppError(err)
		if !ok || appErr.Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("expected application validation error, got %v", err)
		}
		if len(appErr.Violations) != 1 {
			t.Fatalf("expected 1 violation, got %d", len(appErr.Violations))
		}

		got := appErr.Violations[0]
		want := apperrors.Violation{Field: "email", Reason: "REQUIRED", Message: "is required"}
		if got != want {
			t.Fatalf("violation = %+v, want %+v", got, want)
		}
	})

	t.Run("non-proto request passes through", func(t *testing.T) {
		t.Parallel()
		called := false
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			called = true
			return okResp(), nil
		}
		req := connectrpc.NewRequest(&struct{}{})
		_, err := ValidationInterceptor(fakeValidator{err: stderrors.New("ignored")})(next)(context.Background(), req)
		if err != nil || !called {
			t.Fatalf("expected passthrough; called=%v err=%v", called, err)
		}
	})
}

// ---------------------------------------------------------------------------
// DeadlineInterceptor
// ---------------------------------------------------------------------------

func TestValidationEvaluationFailurePreservesCause(t *testing.T) {
	t.Parallel()
	cause := stderrors.New("invalid validation program")
	_, err := ValidationInterceptor(fakeValidator{err: cause})(unaryHandler(okResp(), nil))(context.Background(), newProtoRequest())
	got, ok := apperrors.AsAppError(err)
	if !ok || got.Code != apperrors.ErrCodeInternal || !stderrors.Is(got, cause) || got.Message == cause.Error() {
		t.Fatalf("evaluation failure lost its safe classification or cause: %v", err)
	}
}

func TestDecodedRemoteFailureIsNotForwarded(t *testing.T) {
	t.Parallel()
	remote := mustDecode(t, mustEncode(t, apperrors.New(apperrors.ErrCodeInternal, "upstream private detail"), testDomain))
	handler := NormalizingInterceptor(nil).WrapUnary(unaryHandler(nil, remote))
	_, err := handler(context.Background(), newProtoRequest())
	if strings.Contains(err.Error(), "upstream private detail") {
		t.Fatalf("remote detail crossed the public boundary: %v", err)
	}
	if connectrpc.CodeOf(err) != connectrpc.CodeInternal {
		t.Fatalf("unexpected code: %v", err)
	}
}

func TestDeadlineClampsLongerClientBudget(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	start := time.Now()
	_, err := DeadlineInterceptor(time.Hour)(func(callCtx context.Context, _ connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		deadline, ok := callCtx.Deadline()
		if !ok || deadline.Before(start.Add(time.Hour)) || deadline.After(time.Now().Add(time.Hour)) {
			t.Fatalf("deadline not clamped to server maximum: %v", deadline)
		}
		return okResp(), nil
	})(ctx, newProtoRequest())
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeadlineInterceptor(t *testing.T) {
	t.Parallel()

	t.Run("disabled when max is non-positive", func(t *testing.T) {
		t.Parallel()
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			if _, ok := ctx.Deadline(); ok {
				t.Error("expected no deadline")
			}
			return okResp(), nil
		}
		if _, err := DeadlineInterceptor(0)(next)(context.Background(), newProtoRequest()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("applies max when client sends none", func(t *testing.T) {
		t.Parallel()
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected a bounded deadline")
			}
			if remaining := time.Until(deadline); remaining > time.Minute {
				t.Errorf("deadline too far out: %v", remaining)
			}
			return okResp(), nil
		}
		if _, err := DeadlineInterceptor(30*time.Second)(next)(context.Background(), newProtoRequest()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("keeps a tighter client deadline", func(t *testing.T) {
		t.Parallel()
		clientDeadline := time.Now().Add(2 * time.Second)
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			deadline, _ := ctx.Deadline()
			if !deadline.Equal(clientDeadline) {
				t.Errorf("deadline = %v, want client's %v", deadline, clientDeadline)
			}
			return okResp(), nil
		}
		ctx, cancel := context.WithDeadline(context.Background(), clientDeadline)
		defer cancel()
		if _, err := DeadlineInterceptor(time.Hour)(next)(ctx, newProtoRequest()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("blocked dependency ends at the server maximum", func(t *testing.T) {
		t.Parallel()
		next := func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
			<-ctx.Done()
			return nil, apperrors.Normalize(ctx.Err())
		}
		chain := NormalizingInterceptor(nil).WrapUnary(DeadlineInterceptor(50 * time.Millisecond)(next))
		_, err := chain(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeDeadlineExceeded {
			t.Fatalf("code = %v, want deadline_exceeded", connectrpc.CodeOf(err))
		}
	})
}

// ---------------------------------------------------------------------------
// Composed chain
// ---------------------------------------------------------------------------

func TestComposedChain(t *testing.T) {
	t.Parallel()
	normalizing := NormalizingInterceptor(nil)

	t.Run("auth failure stays unauthenticated", func(t *testing.T) {
		t.Parallel()
		auth, buildErr := AuthInterceptor(getTestClaims)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		handler := normalizing.WrapUnary(auth.WrapUnary(unaryHandler(okResp(), nil)))
		_, err := handler(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeUnauthenticated {
			t.Fatalf("code = %v, want unauthenticated", connectrpc.CodeOf(err))
		}
	})

	t.Run("validation failure surfaces violations", func(t *testing.T) {
		t.Parallel()
		v := fakeValidator{err: newValidationError("name", "required", "is required")}
		handler := normalizing.WrapUnary(ValidationInterceptor(v).WrapUnary(unaryHandler(okResp(), nil)))
		_, err := handler(context.Background(), newProtoRequest())
		if connectrpc.CodeOf(err) != connectrpc.CodeInvalidArgument {
			t.Fatalf("code = %v, want invalid_argument", connectrpc.CodeOf(err))
		}
		if len(mustDecode(t, err).Violations) != 1 {
			t.Fatal("expected violations to survive the chain")
		}
	})
}

// ---------------------------------------------------------------------------
// LoggingInterceptor
// ---------------------------------------------------------------------------

func TestLoggingInterceptor(t *testing.T) {
	t.Parallel()
	log := logging.MustNew(&logging.Config{Level: "debug", Format: "json", Output: logging.OutputStdout()}, "connect-test")
	req := newProtoRequest()
	wantResp := okResp()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := LoggingInterceptor(log)(unaryHandler(wantResp, nil))(context.Background(), req)
		if err != nil || resp != wantResp {
			t.Fatalf("resp=%v err=%v", resp, err)
		}
	})

	t.Run("error passes through", func(t *testing.T) {
		t.Parallel()
		wantErr := connectrpc.NewError(connectrpc.CodeUnavailable, stderrors.New("down"))
		_, err := LoggingInterceptor(log)(unaryHandler(wantResp, wantErr))(context.Background(), req)
		if !stderrors.Is(err, wantErr) {
			t.Fatalf("error = %v, want original", err)
		}
	})
}
