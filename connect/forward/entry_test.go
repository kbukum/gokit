package forward_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kbukum/gokit/connect/client"
	"github.com/kbukum/gokit/connect/forward"
	apperrors "github.com/kbukum/gokit/errors"
)

func descriptor(t *testing.T, idempotency descriptorpb.MethodOptions_IdempotencyLevel, clientStream, serverStream bool) protoreflect.MethodDescriptor {
	t.Helper()
	files := new(protoregistry.Files)
	if err := files.RegisterFile(wrapperspb.File_google_protobuf_wrappers_proto); err != nil {
		t.Fatal(err)
	}
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("forward_test.proto"), Package: proto.String("forward.test.v1"), Syntax: proto.String("proto3"),
		Dependency: []string{"google/protobuf/wrappers.proto"},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("EchoService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("Echo"), InputType: proto.String(".google.protobuf.StringValue"), OutputType: proto.String(".google.protobuf.StringValue"),
				ClientStreaming: proto.Bool(clientStream), ServerStreaming: proto.Bool(serverStream),
				Options: &descriptorpb.MethodOptions{IdempotencyLevel: &idempotency},
			}},
		}},
	}, files)
	if err != nil {
		t.Fatal(err)
	}
	return file.Services().Get(0).Methods().Get(0)
}

func entry(t *testing.T, level descriptorpb.MethodOptions_IdempotencyLevel) forward.Entry {
	t.Helper()
	e, err := forward.NewUnary[wrapperspb.StringValue, wrapperspb.StringValue](descriptor(t, level, false, false))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestNewUnary(t *testing.T) {
	t.Parallel()
	e := entry(t, descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN)
	if got := e.Procedure(); got != "/forward.test.v1.EchoService/Echo" {
		t.Fatalf("procedure = %q", got)
	}
}

func TestNewUnaryRejectsInvalidDescriptors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		method protoreflect.MethodDescriptor
	}{
		{"nil", nil},
		{"client stream", descriptor(t, 0, true, false)},
		{"server stream", descriptor(t, 0, false, true)},
		{"bidirectional stream", descriptor(t, 0, true, true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := forward.NewUnary[wrapperspb.StringValue, wrapperspb.StringValue](tc.method)
			assertAppError(t, err)
		})
	}
	t.Run("request mismatch", func(t *testing.T) {
		_, err := forward.NewUnary[emptypb.Empty, wrapperspb.StringValue](descriptor(t, 0, false, false))
		assertAppError(t, err)
	})
	t.Run("response mismatch", func(t *testing.T) {
		_, err := forward.NewUnary[wrapperspb.StringValue, emptypb.Empty](descriptor(t, 0, false, false))
		assertAppError(t, err)
	})
}

func assertAppError(t *testing.T, err error) {
	t.Helper()
	if _, ok := errors.AsType[*apperrors.AppError](err); !ok {
		t.Fatalf("error = %v, want typed application error", err)
	}
}

type httpCall func(*http.Request) (*http.Response, error)

func (f httpCall) Do(r *http.Request) (*http.Response, error) { return f(r) }

func echo(_ context.Context, in proto.Message) (proto.Message, error) { return in, nil }

func TestUnaryWireAndHooks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		level  descriptorpb.MethodOptions_IdempotencyLevel
		opts   []connectrpc.ClientOption
		method string
	}{
		{"protobuf", descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN, nil, http.MethodPost},
		{"json", descriptorpb.MethodOptions_IDEMPOTENT, []connectrpc.ClientOption{connectrpc.WithProtoJSON()}, http.MethodPost},
		{"get", descriptorpb.MethodOptions_NO_SIDE_EFFECTS, []connectrpc.ClientOption{connectrpc.WithHTTPGet()}, http.MethodGet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := entry(t, tc.level)
			handler, err := e.Handler(echo)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			httpClient := httpCall(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != tc.method || req.URL.Path != e.Procedure() || req.Header.Get("X-Test-Policy") != "allowed" {
					t.Errorf("unexpected request: %s %s, policy %q", req.Method, req.URL.Path, req.Header.Get("X-Test-Policy"))
				}
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, req)
				return out.Result(), nil
			})
			send, err := e.Client(httpClient, client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test", Timeout: time.Second}, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}
			in := wrapperspb.String("preserved")
			if tc.name == "protobuf" {
				in.ProtoReflect().SetUnknown([]byte{0x10, 0x07})
			}
			out, err := send(t.Context(), in, func(_ context.Context, headers http.Header) error {
				headers.Set("X-Test-Policy", "allowed")
				return nil
			})
			if err != nil || !proto.Equal(in, out) || calls != 1 {
				t.Fatalf("out = %v, error = %v, calls = %d", out, err, calls)
			}
		})
	}
}

func TestUnaryClientRefusesInvalidConstruction(t *testing.T) {
	t.Parallel()
	e := entry(t, 0)
	httpClient := httpCall(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected call")
		return nil, errors.New("unexpected call")
	})
	for _, tc := range []struct {
		name string
		http connectrpc.HTTPClient
		cfg  client.Config
	}{
		{"nil client", nil, client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test"}},
		{"typed nil client", (*http.Client)(nil), client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test"}},
		{"missing URL", httpClient, client.Config{AllowInsecureDev: true}},
		{"credentials", httpClient, client.Config{AllowInsecureDev: true, BaseURL: "http://private-user:private-password@peer.test"}},
		{"query", httpClient, client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test?secret=private-password"}},
		{"negative timeout", httpClient, client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test", Timeout: -time.Second}},
		{"unbounded unary", httpClient, client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test", NoTimeout: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := e.Client(tc.http, tc.cfg)
			assertAppError(t, err)
			if strings.Contains(err.Error(), "private-password") {
				t.Fatal("configuration error exposes credentials")
			}
		})
	}
	if _, err := e.Handler(nil); err == nil {
		t.Fatal("nil handler callback accepted")
	}
}

func TestUnaryRefusalAndNoRetry(t *testing.T) {
	t.Parallel()
	e := entry(t, 0)
	refusal := connectrpc.NewError(connectrpc.CodePermissionDenied, errors.New("refused"))
	detail, err := connectrpc.NewErrorDetail(wrapperspb.String("safe-detail"))
	if err != nil {
		t.Fatal(err)
	}
	refusal.AddDetail(detail)
	refusal.Meta().Set("Retry-After", "2")
	handler, err := e.Handler(func(context.Context, proto.Message) (proto.Message, error) { return nil, refusal })
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	send, err := e.Client(httpCall(func(req *http.Request) (*http.Response, error) {
		calls++
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out.Result(), nil
	}), client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = send(t.Context(), wrapperspb.String("input"), nil)
	got, ok := errors.AsType[*connectrpc.Error](err)
	if !ok || got.Code() != refusal.Code() || got.Meta().Get("Retry-After") != "2" || len(got.Details()) != 1 || calls != 1 {
		t.Fatalf("error = %v, calls = %d", err, calls)
	}
}

func TestUnaryClientRejectsMessagesAndHookFailure(t *testing.T) {
	t.Parallel()
	e := entry(t, 0)
	calls := 0
	send, err := e.Client(httpCall(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected HTTP call")
	}), client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []proto.Message{nil, (*wrapperspb.StringValue)(nil), &emptypb.Empty{}} {
		_, err := send(t.Context(), in, nil)
		assertAppError(t, err)
	}
	cause := errors.New("policy refused")
	_, err = send(t.Context(), wrapperspb.String("input"), func(context.Context, http.Header) error { return cause })
	if !errors.Is(err, cause) || calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, calls)
	}
}

func TestUnaryRejectsWrongResponse(t *testing.T) {
	t.Parallel()
	for _, response := range []proto.Message{nil, (*wrapperspb.StringValue)(nil), &emptypb.Empty{}} {
		e := entry(t, 0)
		handler, err := e.Handler(func(context.Context, proto.Message) (proto.Message, error) { return response, nil })
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, e.Procedure(), strings.NewReader(`"input"`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %s", out.Code, out.Body.String())
		}
	}
}

func TestUnaryHonorsCancellationAndBudget(t *testing.T) {
	t.Parallel()
	e := entry(t, 0)
	for _, tc := range []struct {
		name     string
		canceled bool
	}{
		{"caller cancellation", true},
		{"peer budget", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if tc.canceled {
					cancel()
				}
				send, err := e.Client(httpCall(func(req *http.Request) (*http.Response, error) {
					if _, ok := req.Context().Deadline(); !ok {
						t.Error("outbound call has no deadline")
					}
					<-req.Context().Done()
					return nil, req.Context().Err()
				}), client.Config{AllowInsecureDev: true, BaseURL: "http://peer.test", Timeout: time.Millisecond})
				if err != nil {
					t.Fatal(err)
				}
				_, err = send(ctx, wrapperspb.String("input"), nil)
				want := connectrpc.CodeDeadlineExceeded
				if tc.canceled {
					want = connectrpc.CodeCanceled
				}
				if connectrpc.CodeOf(err) != want {
					t.Fatalf("error = %v, want %v", err, want)
				}
			})
		})
	}
}
