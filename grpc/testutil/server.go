package testutil

import (
	"context"
	"errors"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

const (
	// ServiceName is the fully-qualified name of the harness service.
	ServiceName = "gokit.grpc.testutil.Test"
	// MethodUnary is the full method path of the harness unary method, suitable
	// for grpc.ClientConn.Invoke.
	MethodUnary = "/" + ServiceName + "/Unary"

	bufSize = 1024 * 1024
)

var (
	_ component.Component    = (*Server)(nil)
	_ testutil.TestComponent = (*Server)(nil)
)

// UnaryHandler drives the harness unary method. Returning nil makes the call
// succeed with an empty response; returning an error (typically a *status.Status
// error) propagates that status to the client. The handler receives the
// server-side call context, so it can block on ctx.Done to model a call that
// runs until the caller cancels or its deadline passes.
type UnaryHandler func(ctx context.Context) error

// Server is an in-process gRPC server backed by an in-memory bufconn listener.
// It serves from construction; use Dial to obtain a client connection wired to
// the in-memory transport and SetUnaryHandler to control how MethodUnary
// responds.
type Server struct {
	mu      sync.Mutex
	lis     *bufconn.Listener
	gs      *grpc.Server
	handler UnaryHandler
	started bool
}

// NewServer creates and starts an in-process gRPC server. The server is
// immediately ready for Dial; the default unary handler returns success.
func NewServer() *Server {
	s := &Server{}
	s.serve()
	return s
}

// serve builds a fresh listener and grpc.Server and begins serving. Callers hold
// s.mu (or construct before any concurrency); the serve goroutine closes over
// local copies so a later restart never races the running server.
func (s *Server) serve() {
	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	gs.RegisterService(&serviceDesc, s)
	s.lis = lis
	s.gs = gs
	s.started = true
	go func() {
		// Serve returns when the listener is closed by Stop; that is a normal
		// shutdown, not a failure.
		_ = gs.Serve(lis)
	}()
}

// SetUnaryHandler installs the handler invoked by MethodUnary. Passing nil
// restores the default success behavior.
func (s *Server) SetUnaryHandler(h UnaryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// Dial returns a client connection wired to the in-memory transport. Additional
// dial options are appended after the harness defaults (insecure credentials and
// the bufconn dialer), so a caller may layer interceptors or call options on top.
func (s *Server) Dial(opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	s.mu.Lock()
	lis := s.lis
	s.mu.Unlock()

	dialOpts := append([]grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, opts...)
	return grpc.NewClient("passthrough:///bufconn", dialOpts...)
}

// Invoke performs the harness unary call over conn using the default proto codec.
func (s *Server) Invoke(ctx context.Context, conn *grpc.ClientConn, opts ...grpc.CallOption) error {
	return conn.Invoke(ctx, MethodUnary, new(emptypb.Empty), new(emptypb.Empty), opts...)
}

// currentHandler returns the installed handler or a success default.
func (s *Server) currentHandler() UnaryHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler
}

func (s *Server) handleUnary(
	srv any,
	ctx context.Context,
	dec func(any) error,
	interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	run := func(ctx context.Context, _ any) (any, error) {
		if h := s.currentHandler(); h != nil {
			if err := h(ctx); err != nil {
				return nil, err
			}
		}
		return new(emptypb.Empty), nil
	}
	if interceptor == nil {
		return run(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: MethodUnary}
	return interceptor(ctx, in, info, run)
}

var serviceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*any)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(
				srv any,
				ctx context.Context,
				dec func(any) error,
				interceptor grpc.UnaryServerInterceptor,
			) (any, error) {
				return srv.(*Server).handleUnary(srv, ctx, dec, interceptor)
			},
		},
	},
	Metadata: "github.com/kbukum/gokit/grpc/testutil",
}

// --- component.Component ---

// Name returns the component name.
func (s *Server) Name() string { return "grpc-test-server" }

// Start satisfies component.Component. The server serves from construction, so
// Start only restarts a server that was previously stopped.
func (s *Server) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	s.serve()
	return nil
}

// Stop gracefully shuts the server down and closes the listener.
func (s *Server) Stop(_ context.Context) error {
	s.mu.Lock()
	gs, lis, started := s.gs, s.lis, s.started
	s.started = false
	s.mu.Unlock()

	if !started || gs == nil {
		return nil
	}
	gs.GracefulStop()
	if lis != nil {
		return lis.Close()
	}
	return nil
}

// Health reports healthy while the server is serving.
func (s *Server) Health(_ context.Context) component.Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return component.Unhealthy(s.Name(), "not started")
	}
	return component.Healthy(s.Name())
}

// --- testutil.TestComponent ---

// Reset stops the running server and starts a fresh one, clearing any installed
// unary handler.
func (s *Server) Reset(ctx context.Context) error {
	if err := s.Stop(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = nil
	s.serve()
	return nil
}

// Snapshot is not supported: the harness holds no persistent state to capture.
func (s *Server) Snapshot(_ context.Context) (any, error) {
	return nil, errors.ErrUnsupported
}

// Restore is not supported: the harness holds no persistent state to restore.
func (s *Server) Restore(_ context.Context, _ any) error {
	return errors.ErrUnsupported
}
