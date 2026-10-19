package forward

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	goconnect "github.com/kbukum/gokit/connect"
	"github.com/kbukum/gokit/connect/client"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Call handles a decoded message and returns the method's concrete response type.
type Call func(context.Context, proto.Message) (proto.Message, error)

// HeaderHook supplies explicitly selected outbound headers or refuses the call. It performs no blocking I/O and never copies untrusted headers indiscriminately.
type HeaderHook func(context.Context, http.Header) error

// Send invokes a unary method. A nil hook adds no application-specific headers.
type Send func(context.Context, proto.Message, HeaderHook) (proto.Message, error)

// Entry is an immutable typed unary method usable in a heterogeneous route catalog.
type Entry interface {
	Procedure() string
	Handler(Call, ...connectrpc.HandlerOption) (http.Handler, error)
	Client(connectrpc.HTTPClient, client.Config, ...connectrpc.ClientOption) (Send, error)
}

type message[T any] interface {
	*T
	proto.Message
}

// NewUnary validates a generated method against its concrete request and response types.
func NewUnary[Req, Res any, PReq message[Req], PRes message[Res]](method protoreflect.MethodDescriptor) (Entry, error) {
	if util.IsNil(method) {
		return nil, apperrors.InvalidInput("method", "A method descriptor is required")
	}
	service, ok := method.Parent().(protoreflect.ServiceDescriptor)
	if !ok || method.IsStreamingClient() || method.IsStreamingServer() {
		return nil, apperrors.InvalidInput("method", "A unary service method is required")
	}
	var req PReq = new(Req)
	var res PRes = new(Res)
	if method.Input() != req.ProtoReflect().Descriptor() || method.Output() != res.ProtoReflect().Descriptor() {
		return nil, apperrors.InvalidInput("method", "Request and response types must match the method descriptor")
	}
	level := connectrpc.IdempotencyUnknown
	if opts, ok := method.Options().(*descriptorpb.MethodOptions); ok {
		switch opts.GetIdempotencyLevel() {
		case descriptorpb.MethodOptions_IDEMPOTENCY_UNKNOWN:
			level = connectrpc.IdempotencyUnknown
		case descriptorpb.MethodOptions_NO_SIDE_EFFECTS:
			level = connectrpc.IdempotencyNoSideEffects
		case descriptorpb.MethodOptions_IDEMPOTENT:
			level = connectrpc.IdempotencyIdempotent
		}
	}
	return &unary[Req, Res, PReq, PRes]{
		procedure: "/" + string(service.FullName()) + "/" + string(method.Name()),
		method:    method,
		level:     level,
	}, nil
}

type unary[Req, Res any, PReq message[Req], PRes message[Res]] struct {
	procedure string
	method    protoreflect.MethodDescriptor
	level     connectrpc.IdempotencyLevel
}

func (u *unary[Req, Res, PReq, PRes]) Procedure() string { return u.procedure }

func (u *unary[Req, Res, PReq, PRes]) Handler(call Call, opts ...connectrpc.HandlerOption) (http.Handler, error) {
	if call == nil {
		return nil, apperrors.InvalidInput("handler", "A forwarding callback is required")
	}
	opts = append(slices.Clone(opts), connectrpc.WithSchema(u.method), connectrpc.WithIdempotency(u.level))
	opts = append([]connectrpc.HandlerOption{connectrpc.WithInterceptors(goconnect.NormalizingInterceptor(nil))}, opts...)
	return connectrpc.NewUnaryHandler(u.procedure, func(ctx context.Context, req *connectrpc.Request[Req]) (*connectrpc.Response[Res], error) {
		out, err := call(ctx, PReq(req.Msg))
		if err != nil {
			return nil, err
		}
		res, ok := out.(PRes)
		if !ok || res == nil {
			return nil, apperrors.Internal(errors.New("forwarded response has an unexpected message type"))
		}
		return connectrpc.NewResponse((*Res)(res)), nil
	}, opts...), nil
}

func (u *unary[Req, Res, PReq, PRes]) Client(httpClient connectrpc.HTTPClient, cfg client.Config, opts ...connectrpc.ClientOption) (Send, error) {
	if util.IsNil(httpClient) {
		return nil, apperrors.InvalidInput("client", "An HTTP client is required")
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, apperrors.InvalidInput("client", "Invalid forwarding client configuration").WithCause(err)
	}
	if cfg.BaseURL == "" || cfg.NoTimeout {
		return nil, apperrors.InvalidInput("client", "Unary forwarding requires a base URL and a finite timeout")
	}
	timeout, err := client.UnaryTimeoutInterceptor(cfg.Timeout)
	if err != nil {
		return nil, apperrors.InvalidInput("timeout", "Invalid unary timeout").WithCause(err)
	}
	options := append(client.ClientOptions(cfg), opts...)
	options = append(options, connectrpc.WithSchema(u.method), connectrpc.WithIdempotency(u.level), connectrpc.WithInterceptors(timeout))
	rpc := connectrpc.NewClient[Req, Res](httpClient, strings.TrimRight(cfg.BaseURL, "/")+u.procedure, options...)
	return func(ctx context.Context, in proto.Message, prepare HeaderHook) (proto.Message, error) {
		msg, ok := in.(PReq)
		if !ok || msg == nil {
			return nil, apperrors.InvalidInput("request", "Request has an unexpected message type")
		}
		if err := ctx.Err(); err != nil {
			code := connectrpc.Code(apperrors.RPCCodeFor(apperrors.Normalize(err).Code))
			return nil, connectrpc.NewError(code, err)
		}
		req := connectrpc.NewRequest((*Req)(msg))
		if prepare != nil {
			if err := prepare(ctx, req.Header()); err != nil {
				return nil, err
			}
		}
		resp, err := rpc.CallUnary(ctx, req)
		if err != nil {
			return nil, err
		}
		return PRes(resp.Msg), nil
	}, nil
}
