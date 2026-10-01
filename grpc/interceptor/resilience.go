package interceptor

import (
	"context"

	"google.golang.org/grpc"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/resilience"
)

type idempotentCall struct{ grpc.EmptyCallOption }

// Idempotent marks an operation safe to repeat. A transient error alone never grants permission to retry.
func Idempotent() grpc.CallOption { return idempotentCall{} }

// UnaryClientResilienceInterceptor applies the shared resilience policy to unary client calls.
func UnaryClientResilienceInterceptor(policy *resilience.Policy) grpc.UnaryClientInterceptor {
	var retry *resilience.RetryConfig
	if policy != nil && policy.Retry != nil {
		config := *policy.Retry
		if config.RetryIf == nil {
			config.RetryIf = grpccfg.IsRetryable
		}
		if config.MinimumDelay == nil {
			config.MinimumDelay = grpccfg.RetryDelay
		}
		retry = &config
	}
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var callRetry *resilience.RetryConfig
		for _, option := range opts {
			if _, ok := option.(idempotentCall); ok {
				callRetry = retry
			}
		}
		_, err := resilience.ExecuteWithRetry(ctx, policy, callRetry, func(callCtx context.Context) (struct{}, error) {
			return struct{}{}, invoker(callCtx, method, req, reply, cc, opts...)
		})
		return err
	}
}
