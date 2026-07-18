package interceptor

import (
	"context"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/kbukum/gokit/logging"
)

// UnaryClientLoggingInterceptor returns a unary client interceptor that logs each RPC call with method, duration, and status.
func UnaryClientLoggingInterceptor(log *logging.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		service := path.Dir(method)[1:]
		methodName := path.Base(method)

		log.DebugCtx(ctx, "gRPC call started", map[string]any{
			"service": service,
			"method":  methodName,
			"target":  cc.Target(),
		})

		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		fields := map[string]any{
			"service":     service,
			"method":      methodName,
			"duration_ms": duration.Milliseconds(),
			"target":      cc.Target(),
		}

		if err != nil {
			st := status.Convert(err)
			fields["status"] = st.Code().String()
			fields["error"] = st.Message()
			log.ErrorCtx(ctx, "gRPC call failed", fields)
		} else {
			fields["status"] = "OK"
			log.DebugCtx(ctx, "gRPC call completed", fields)
		}

		return err
	}
}

// StreamClientLoggingInterceptor returns a stream client interceptor that logs stream establishment with method, duration, and status.
func StreamClientLoggingInterceptor(log *logging.Logger) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()
		service := path.Dir(method)[1:]
		methodName := path.Base(method)

		log.DebugCtx(ctx, "gRPC stream started", map[string]any{
			"service":        service,
			"method":         methodName,
			"target":         cc.Target(),
			"server_streams": desc.ServerStreams,
			"client_streams": desc.ClientStreams,
		})

		stream, err := streamer(ctx, desc, cc, method, opts...)
		duration := time.Since(start)

		fields := map[string]any{
			"service":     service,
			"method":      methodName,
			"duration_ms": duration.Milliseconds(),
			"target":      cc.Target(),
		}

		if err != nil {
			st := status.Convert(err)
			fields["status"] = st.Code().String()
			fields["error"] = st.Message()
			log.ErrorCtx(ctx, "gRPC stream failed", fields)
		} else {
			fields["status"] = "STARTED"
			log.DebugCtx(ctx, "gRPC stream established", fields)
		}

		return stream, err
	}
}
