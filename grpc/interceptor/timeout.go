package interceptor

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// UnaryClientTimeoutInterceptor returns a unary client interceptor that applies
// a default timeout to calls that do not already have a deadline.
func UnaryClientTimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if _, ok := ctx.Deadline(); !ok && timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
