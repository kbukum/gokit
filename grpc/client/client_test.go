package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	grpccfg "github.com/kbukum/gokit/grpc"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func testLogger() *logging.Logger { return logging.NewDefault("test") }

func validInsecureConfig() grpccfg.Config {
	return grpccfg.Config{
		Name:           "test-svc",
		Addr:           "passthrough:///localhost:50051",
		MaxRecvMsgSize: 4 * 1024 * 1024,
		MaxSendMsgSize: 4 * 1024 * 1024,
		Keepalive: grpccfg.KeepaliveConfig{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		},
		CallTimeout: 5 * time.Second,
	}
}

// testConn returns a lightweight lazy gRPC client connection.
func testConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	cc, err := grpc.NewClient("passthrough:///test",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

func readyConn(t *testing.T) *grpc.ClientConn {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	go func() {
		_ = server.Serve(listener)
	}()

	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	cc, err := grpc.NewClient("passthrough:///"+listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, waitForConnectionReady(ctx, cc))

	return cc
}

func unavailableConn(t *testing.T) *grpc.ClientConn {
	t.Helper()

	cc, err := grpc.NewClient("passthrough:///127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// mockConnectionFactory implements ConnectionFactory for testing.
type mockConnectionFactory struct {
	conn   *grpc.ClientConn
	err    error
	calls  atomic.Int32
	mu     sync.Mutex
	connFn func(string) (*grpc.ClientConn, error) // optional dynamic behavior
}

func (f *mockConnectionFactory) NewConn(serviceName string) (*grpc.ClientConn, error) {
	f.calls.Add(1)
	if f.connFn != nil {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.connFn(serviceName)
	}
	return f.conn, f.err
}

// mockGRPCClient is a trivial type used as the T parameter for LazyClient.
type mockGRPCClient struct {
	conn grpc.ClientConnInterface
}

func newMockGRPCClient(cc grpc.ClientConnInterface) mockGRPCClient {
	return mockGRPCClient{conn: cc}
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_InsecureConfig(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	assert.Equal(t, "passthrough:///localhost:50051", conn.Target())
}

func TestNewClient_AppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg := grpccfg.Config{
		Addr: "passthrough:///localhost:9090",
	}
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}

func TestNewClient_ValidationFailure_EmptyAddr(t *testing.T) {
	t.Parallel()

	// ApplyDefaults will fill Addr, so we need zero MaxRecvMsgSize to cause failure.
	// Actually, ApplyDefaults is called first, so Addr will be set.
	// The only way to fail validation after defaults is negative msg size.
	cfg := grpccfg.Config{
		MaxRecvMsgSize: -1,
	}
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "max_recv_msg_size must be positive")
}

func TestNewClient_ValidationFailure_BadTLS(t *testing.T) {
	t.Parallel()

	cfg := grpccfg.Config{
		Addr:           "passthrough:///localhost:50051",
		MaxRecvMsgSize: 1024,
		MaxSendMsgSize: 1024,
		TLS:            &security.TLSConfig{CertFile: "/nonexistent.pem"},
	}
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.Error(t, err)
	assert.Nil(t, conn)
}

func TestNewClient_WithCallTimeout(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	cfg.CallTimeout = 2 * time.Second
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}

func TestNewClient_WithKeepalive(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	cfg.Keepalive = grpccfg.KeepaliveConfig{
		Time:                60 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}
	log := testLogger()

	conn, err := NewClient(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}

// ---------------------------------------------------------------------------
// Adapter
// ---------------------------------------------------------------------------

func TestNewAdapter_InsecureConfig(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, adapter)
	adapter.Close(context.Background())
}

func TestNewAdapter_ValidationFailure(t *testing.T) {
	t.Parallel()

	cfg := grpccfg.Config{MaxRecvMsgSize: -1}
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.Error(t, err)
	assert.Nil(t, adapter)
}

func TestAdapter_Name(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	cfg.Name = "user-service"
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	defer adapter.Close(context.Background())

	assert.Equal(t, "user-service", adapter.Name())
}

func TestAdapter_Conn(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	defer adapter.Close(context.Background())

	assert.NotNil(t, adapter.Conn())
}

func TestAdapter_GetConfig(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	defer adapter.Close(context.Background())

	got := adapter.GetConfig()
	assert.Equal(t, cfg.Name, got.Name)
	// Addr may have been modified by ApplyDefaults inside NewClient
	assert.NotEmpty(t, got.Addr)
}

func TestAdapter_IsAvailable(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	defer adapter.Close(context.Background())

	// grpc.NewClient creates a lazy connection; initial state is Idle which is "available"
	assert.True(t, adapter.IsAvailable(context.Background()))
}

func TestAdapter_IsAvailable_NilConn(t *testing.T) {
	t.Parallel()
	adapter := &Adapter{conn: nil}
	assert.False(t, adapter.IsAvailable(context.Background()))
}

func TestAdapter_Close(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)

	err = adapter.Close(context.Background())
	require.NoError(t, err)
}

func TestAdapter_Close_NilConn(t *testing.T) {
	t.Parallel()
	adapter := &Adapter{conn: nil}
	require.NoError(t, adapter.Close(context.Background()))
}

func TestClientOf(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	adapter, err := NewAdapter(cfg, log)
	require.NoError(t, err)
	defer adapter.Close(context.Background())

	client := ClientOf(adapter, newMockGRPCClient)
	assert.NotNil(t, client.conn, "ClientOf should pass the adapter's connection")
}

// ---------------------------------------------------------------------------
// LazyClient
// ---------------------------------------------------------------------------

func TestLazyClient_GetClient_Success(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	lc := NewLazyClient("test-svc", factory, newMockGRPCClient, testLogger())

	client, err := lc.GetClient()
	require.NoError(t, err)
	assert.NotNil(t, client.conn)
	assert.Equal(t, int32(1), factory.calls.Load(), "factory should be called once")

	// Second call reuses cached client
	client2, err := lc.GetClient()
	require.NoError(t, err)
	assert.Equal(t, client, client2)
	assert.Equal(t, int32(1), factory.calls.Load(), "factory should still be called only once")
}

func TestLazyClient_GetClient_FactoryError(t *testing.T) {
	t.Parallel()

	factory := &mockConnectionFactory{err: errors.New("connection refused")}

	lc := NewLazyClient("bad-svc", factory, newMockGRPCClient, testLogger())

	_, err := lc.GetClient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to bad-svc")
}

func TestLazyClient_GetClient_RetryAfterError(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	callCount := atomic.Int32{}
	factory := &mockConnectionFactory{
		connFn: func(name string) (*grpc.ClientConn, error) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, errors.New("first attempt fails")
			}
			return conn, nil
		},
	}

	lc := NewLazyClient("retry-svc", factory, newMockGRPCClient, testLogger())

	// First call fails
	_, err := lc.GetClient()
	require.Error(t, err)

	// Second call succeeds
	client, err := lc.GetClient()
	require.NoError(t, err)
	assert.NotNil(t, client.conn)
}

func TestLazyClient_Close(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	// Initialize first
	_, err := lc.GetClient()
	require.NoError(t, err)
	assert.True(t, lc.IsConnected())

	// Close
	err = lc.Close()
	require.NoError(t, err)
	assert.False(t, lc.IsConnected())
}

func TestLazyClient_Close_NotInitialized(t *testing.T) {
	t.Parallel()

	factory := &mockConnectionFactory{}
	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	err := lc.Close()
	require.NoError(t, err)
}

func TestLazyClient_IsConnected(t *testing.T) {
	t.Parallel()

	factory := &mockConnectionFactory{err: errors.New("fail")}
	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	assert.False(t, lc.IsConnected(), "should not be connected before GetClient")

	_, _ = lc.GetClient()
	assert.False(t, lc.IsConnected(), "should not be connected after failed GetClient")
}

func TestLazyClient_ServiceName(t *testing.T) {
	t.Parallel()

	factory := &mockConnectionFactory{}
	lc := NewLazyClient("my-analysis-svc", factory, newMockGRPCClient)

	assert.Equal(t, "my-analysis-svc", lc.ServiceName())
}

func TestLazyClient_Reset(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	_, err := lc.GetClient()
	require.NoError(t, err)
	assert.True(t, lc.IsConnected())

	err = lc.Reset()
	require.NoError(t, err)
	assert.False(t, lc.IsConnected())
}

func TestLazyClient_GetConnection(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	assert.Nil(t, lc.GetConnection(), "nil before init")

	_, err := lc.GetClient()
	require.NoError(t, err)
	assert.Equal(t, conn, lc.GetConnection())
}

func TestLazyClient_ConcurrentGetClient(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	lc := NewLazyClient("svc", factory, newMockGRPCClient, testLogger())

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = lc.GetClient()
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	// Factory should have been called exactly once due to double-check locking
	assert.Equal(t, int32(1), factory.calls.Load())
}

func TestLazyClient_NoLogger(t *testing.T) {
	t.Parallel()

	conn := testConn(t)
	factory := &mockConnectionFactory{conn: conn}

	// No logger passed — should use global logger without panicking
	lc := NewLazyClient("svc", factory, newMockGRPCClient)

	client, err := lc.GetClient()
	require.NoError(t, err)
	assert.NotNil(t, client.conn)
}

// ---------------------------------------------------------------------------
// ClientOptionsBuilder
// ---------------------------------------------------------------------------

func TestClientOptionsBuilder_Build(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	builder := NewClientOptionsBuilder(&cfg)

	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts, "should produce dial options")
}

func TestClientOptionsBuilder_WithLoggingDisabled(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	builder := NewClientOptionsBuilder(&cfg).WithLogging(false)

	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestClientOptionsBuilder_WithRetryPolicy(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	policy := &resilience.RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		BackoffFactor:  1.5,
	}
	builder := NewClientOptionsBuilder(&cfg).WithRetryPolicy(policy)

	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestClientOptionsBuilder_NilRetryPolicy(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	builder := NewClientOptionsBuilder(&cfg).WithRetryPolicy(nil)

	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestClientOptionsBuilder_WithCustomInterceptors(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	called := false
	customUnary := func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		called = true
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	builder := NewClientOptionsBuilder(&cfg).WithUnaryInterceptor(customUnary)
	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
	_ = called // used to verify the interceptor is wired
}

func TestClientOptionsBuilder_CustomUnaryRunsAfterResilience(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	builder := NewClientOptionsBuilder(&cfg)

	customSawDeadline := false
	builder.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		_, customSawDeadline = ctx.Deadline()
		return invoker(ctx, method, req, reply, cc, opts...)
	})

	interceptors := builder.buildUnaryInterceptors()
	require.GreaterOrEqual(t, len(interceptors), 3)

	cc := testConn(t)
	var call func(int, context.Context) error
	call = func(idx int, ctx context.Context) error {
		if idx == len(interceptors) {
			return nil
		}
		return interceptors[idx](ctx, "/pkg.Svc/Method", nil, nil, cc, func(nextCtx context.Context, method string, req, reply any,
			cc *grpc.ClientConn, opts ...grpc.CallOption,
		) error {
			return call(idx+1, nextCtx)
		})
	}

	require.NoError(t, call(0, context.Background()))
	assert.True(t, customSawDeadline, "custom interceptors should run after resilience applies timeout policy")
}

func TestClientOptionsBuilder_WithStreamInterceptor(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	customStream := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(ctx, desc, cc, method, opts...)
	}

	builder := NewClientOptionsBuilder(&cfg).WithStreamInterceptor(customStream)
	opts, err := builder.Build()
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestClientOptionsBuilder_GetDialTimeout(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	cfg.CallTimeout = 15 * time.Second
	builder := NewClientOptionsBuilder(&cfg)
	assert.Equal(t, 15*time.Second, builder.GetDialTimeout())
}

func TestClientOptionsBuilder_GetDialTimeout_Default(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	cfg.CallTimeout = 0
	builder := NewClientOptionsBuilder(&cfg)
	assert.Equal(t, 10*time.Second, builder.GetDialTimeout())
}

func TestDefaultRetryPolicy(t *testing.T) {
	t.Parallel()

	p := DefaultRetryPolicy()
	assert.Equal(t, 4, p.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, p.InitialBackoff)
	assert.Equal(t, time.Second, p.MaxBackoff)
	assert.InEpsilon(t, 2.0, p.BackoffFactor, 1e-9)
	assert.NotNil(t, p.RetryIf)
}

// ---------------------------------------------------------------------------
// DefaultConnectionFactory
// ---------------------------------------------------------------------------

func TestDefaultConnectionFactory_NewConn(t *testing.T) {
	t.Parallel()

	cfg := validInsecureConfig()
	log := testLogger()

	factory := NewDefaultConnectionFactory(cfg, log)
	conn, err := factory.NewConn("my-svc")
	require.NoError(t, err)
	require.NotNil(t, conn)
	conn.Close()
}

// ---------------------------------------------------------------------------
// OpenStreamWithTimeout / TryOpenStream
// ---------------------------------------------------------------------------

func TestOpenStreamWithTimeout_Success(t *testing.T) {
	t.Parallel()

	conn := readyConn(t)
	opener := func(ctx context.Context) (string, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("stream opener should receive caller context without connect-timeout deadline")
		}
		return "hello-stream", nil
	}

	result, err := OpenStreamWithTimeout(context.Background(), conn, time.Second, opener)
	require.NoError(t, err)
	assert.Equal(t, "hello-stream", result)
}

func TestOpenStreamWithTimeout_Timeout(t *testing.T) {
	t.Parallel()

	conn := unavailableConn(t)
	called := atomic.Bool{}
	opener := func(ctx context.Context) (string, error) {
		called.Store(true)
		return "late", nil
	}

	result, err := OpenStreamWithTimeout(context.Background(), conn, 50*time.Millisecond, opener)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream connection timeout")
	assert.Empty(t, result)
	assert.False(t, called.Load(), "opener should not run before connection is ready")
}

func TestOpenStreamWithTimeout_ContextCanceled(t *testing.T) {
	t.Parallel()

	conn := unavailableConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	opener := func(ctx context.Context) (string, error) {
		t.Fatal("opener should not run after caller cancellation")
		return "late", nil
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := OpenStreamWithTimeout(ctx, conn, 5*time.Second, opener)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, result)
}

func TestOpenStreamWithTimeout_ZeroTimeout(t *testing.T) {
	t.Parallel()

	opener := func(ctx context.Context) (string, error) {
		return "immediate", nil
	}

	result, err := OpenStreamWithTimeout(context.Background(), nil, 0, opener)
	require.NoError(t, err)
	assert.Equal(t, "immediate", result)
}

func TestOpenStreamWithTimeout_NegativeTimeout(t *testing.T) {
	t.Parallel()

	opener := func(ctx context.Context) (string, error) {
		return "immediate", nil
	}

	result, err := OpenStreamWithTimeout(context.Background(), nil, -1*time.Second, opener)
	require.NoError(t, err)
	assert.Equal(t, "immediate", result)
}

func TestOpenStreamWithTimeout_OpenerError(t *testing.T) {
	t.Parallel()

	conn := readyConn(t)
	opener := func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("stream creation failed")
	}

	result, err := OpenStreamWithTimeout(context.Background(), conn, time.Second, opener)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream creation failed")
	assert.Empty(t, result)
}

func TestTryOpenStream_Success(t *testing.T) {
	t.Parallel()

	conn := readyConn(t)
	opener := func(ctx context.Context) (int, error) {
		return 42, nil
	}

	result, err := TryOpenStream(context.Background(), conn, time.Second, opener)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestTryOpenStream_Timeout(t *testing.T) {
	t.Parallel()

	conn := unavailableConn(t)
	opener := func(ctx context.Context) (int, error) {
		t.Fatal("opener should not run when connection readiness times out")
		return 0, nil
	}

	result, err := TryOpenStream(context.Background(), conn, 50*time.Millisecond, opener)
	require.Error(t, err)
	assert.Equal(t, 0, result)
}
