package connect

import (
	"context"
	"net/http"

	connectrpc "connectrpc.com/connect"
	"golang.org/x/net/http/httpguts"

	apperrors "github.com/kbukum/gokit/errors"
)

// HeaderInterceptor returns a client interceptor that sets name to value on every unary and streaming call, replacing
// any value the caller set, such as a service key. It changes nothing on handlers.
func HeaderInterceptor(name, value string) (connectrpc.Interceptor, error) {
	if !httpguts.ValidHeaderFieldName(name) {
		return nil, apperrors.InvalidInput("name", "must be a valid header name")
	}
	if value == "" || !httpguts.ValidHeaderFieldValue(value) {
		return nil, apperrors.InvalidInput("value", "must be a non-empty valid header value")
	}
	return headerInterceptor{name: http.CanonicalHeaderKey(name), value: value}, nil
}

type headerInterceptor struct{ name, value string }

func (h headerInterceptor) WrapUnary(next connectrpc.UnaryFunc) connectrpc.UnaryFunc {
	return func(ctx context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		if req.Spec().IsClient {
			req.Header().Set(h.name, h.value)
		}
		return next(ctx, req)
	}
}

func (h headerInterceptor) WrapStreamingClient(next connectrpc.StreamingClientFunc) connectrpc.StreamingClientFunc {
	return func(ctx context.Context, spec connectrpc.Spec) connectrpc.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set(h.name, h.value)
		return conn
	}
}

func (headerInterceptor) WrapStreamingHandler(next connectrpc.StreamingHandlerFunc) connectrpc.StreamingHandlerFunc {
	return next
}
