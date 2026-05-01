package interceptor

import (
	"context"

	"google.golang.org/grpc"

	"github.com/kbukum/gokit/resilience"
)

// UnaryClientResilienceInterceptor applies the shared resilience policy to
// unary client calls.
func UnaryClientResilienceInterceptor(policy *resilience.Policy) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		_, err := resilience.Execute(ctx, policy, func(callCtx context.Context) (struct{}, error) {
			return struct{}{}, invoker(callCtx, method, req, reply, cc, opts...)
		})
		return err
	}
}
