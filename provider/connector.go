package provider

import (
	"context"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// Connector provides thread-safe deferred initialization for any client type
// with optional resilience (circuit breaker, retry, rate limiting, bulkhead).
//
// The client is created on first use via GetClient() or Call(). If the
// underlying service endpoint changes, call Reset() to force re-creation.
//
// Connector is protocol-agnostic — it works for ConnectRPC, gRPC, HTTP,
// or any client that can be expressed as a factory function.
//
// Usage:
//
//	c := provider.NewConnector(provider.ConnectorConfig[foov1connect.FooServiceClient]{
//	    ServiceName: "foo-service",
//	    Create: func() (foov1connect.FooServiceClient, error) {
//	        url, err := discovery.Resolve("foo-service")
//	        if err != nil { return nil, err }
//	        return foov1connect.NewFooServiceClient(httpClient, url), nil
//	    },
//	    Resilience: &provider.ResilienceConfig{...},
//	})
//
//	resp, err := provider.Call(ctx, c, func(client FooServiceClient) (*Resp, error) {
//	    r, err := client.DoThing(ctx, connect.NewRequest(req))
//	    if err != nil { return nil, err }
//	    return r.Msg, nil
//	})
type Connector[T any] struct {
	serviceName string
	create      func() (T, error)
	onClose     func() error
	state       *ResilienceState

	mu        sync.RWMutex
	client    T
	hasClient bool
}

// ConnectorConfig configures a Connector.
type ConnectorConfig[T any] struct {
	// ServiceName identifies the service for logging.
	ServiceName string

	// Create is the factory function that produces the client.
	// Called once on first use; the result is cached until Close/Reset.
	Create func() (T, error)

	// OnClose is called when the connector is closed or reset.
	// Use this for cleanup (e.g., closing a gRPC connection).
	// Optional — nil means no cleanup.
	OnClose func() error

	// Resilience configures circuit breaker, retry, rate limiting, and bulkhead.
	// Nil means no resilience wrapping.
	Resilience *ResilienceConfig
}

// NewConnector creates a Connector from config.
func NewConnector[T any](cfg ConnectorConfig[T]) *Connector[T] {
	var state *ResilienceState
	if cfg.Resilience != nil {
		state = BuildResilience(*cfg.Resilience)
	}
	return &Connector[T]{
		serviceName: cfg.ServiceName,
		create:      cfg.Create,
		onClose:     cfg.OnClose,
		state:       state,
	}
}

// GetClient returns the client, creating it on first call.
// Thread-safe; only calls Create once even under concurrent access.
func (c *Connector[T]) GetClient() (T, error) {
	c.mu.RLock()
	if c.hasClient {
		client := c.client
		c.mu.RUnlock()
		return client, nil
	}
	c.mu.RUnlock()

	return c.initClient()
}

func (c *Connector[T]) initClient() (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: another goroutine may have initialized while we waited.
	if c.hasClient {
		return c.client, nil
	}

	client, err := c.create()
	if err != nil {
		var zero T
		return zero, err
	}

	c.client = client
	c.hasClient = true

	logger.Info("Connector: client created", map[string]interface{}{
		"service": c.serviceName,
	})

	return c.client, nil
}

// Close resets the connector, calling OnClose if configured.
// The next GetClient/Call will re-create the client.
func (c *Connector[T]) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var closeErr error
	if c.hasClient && c.onClose != nil {
		closeErr = c.onClose()
	}

	var zero T
	c.client = zero
	c.hasClient = false
	return closeErr
}

// Reset is an alias for Close — forces re-creation on next use.
func (c *Connector[T]) Reset() error {
	return c.Close()
}

// ServiceName returns the service name.
func (c *Connector[T]) ServiceName() string {
	return c.serviceName
}

// IsConnected returns true if the client has been created.
func (c *Connector[T]) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hasClient
}

// Call executes fn with the connector's client, wrapped in resilience
// if configured. Both client creation and fn are inside the resilience
// boundary, so creation failures also count toward circuit breaker thresholds.
func Call[C, R any](ctx context.Context, c *Connector[C], fn func(C) (R, error)) (R, error) {
	return ExecuteWithResilience(ctx, c.state, func() (R, error) {
		client, err := c.GetClient()
		if err != nil {
			var zero R
			return zero, err
		}
		return fn(client)
	})
}
