package client

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"

	"github.com/kbukum/gokit/logger"
)

// LazyClient provides thread-safe lazy initialization for gRPC clients.
// It creates the connection and client on first use and reuses them for subsequent calls.
//
// Type parameter T is the gRPC client interface (e.g., analysispb.AnalysisServiceClient).
//
// Usage:
//
//	c := client.NewLazyClient(
//	    "ai-analysis",
//	    factory,
//	    analysispb.NewAnalysisServiceClient,
//	    log,
//	)
//	analysisClient, err := c.GetClient()
type LazyClient[T any] struct {
	serviceName  string
	factory      ConnectionFactory
	createClient func(grpc.ClientConnInterface) T
	log          *logger.Logger

	mu          sync.RWMutex
	conn        *grpc.ClientConn
	client      T
	initialized bool
	lastError   error
}

// NewLazyClient creates a new lazy client for the specified service.
//
// Parameters:
//   - serviceName: The name of the service for discovery and logging
//   - factory: The connection factory to use for creating connections
//   - createClient: Function to create the typed client from a connection
//     (usually the generated New*Client function from protobuf)
//   - log: Optional logger instance; if nil, uses the global logger
func NewLazyClient[T any](
	serviceName string,
	factory ConnectionFactory,
	createClient func(grpc.ClientConnInterface) T,
	log ...*logger.Logger,
) *LazyClient[T] {
	lc := &LazyClient[T]{
		serviceName:  serviceName,
		factory:      factory,
		createClient: createClient,
	}
	if len(log) > 0 && log[0] != nil {
		lc.log = log[0]
	}
	return lc
}

// GetClient returns the gRPC client, initializing the connection if needed.
// This method is thread-safe and will only create one connection even if
// called concurrently from multiple goroutines.
func (c *LazyClient[T]) GetClient() (T, error) {
	// Fast path: check if already initialized
	c.mu.RLock()
	if c.initialized && c.lastError == nil {
		client := c.client
		c.mu.RUnlock()
		return client, nil
	}
	c.mu.RUnlock()

	// Slow path: initialize the client
	return c.initializeClient()
}

// initializeClient performs the actual initialization with proper locking.
func (c *LazyClient[T]) initializeClient() (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check pattern: another goroutine may have initialized while we waited
	if c.initialized && c.lastError == nil {
		return c.client, nil
	}

	// Reset state for fresh initialization attempt
	var zero T
	c.initialized = false
	c.lastError = nil

	c.logDebug("Initializing gRPC client", map[string]interface{}{
		"service": c.serviceName,
	})

	// Create connection via factory (uses discovery if configured)
	conn, err := c.factory.NewConn(c.serviceName)
	if err != nil {
		c.lastError = err
		c.logError("Failed to create gRPC connection", map[string]interface{}{
			"service": c.serviceName,
			"error":   err.Error(),
		})
		return zero, fmt.Errorf("failed to connect to %s: %w", c.serviceName, err)
	}

	// Create typed client using the provided factory function
	client := c.createClient(conn)

	// Store successful initialization
	c.conn = conn
	c.client = client
	c.initialized = true
	c.lastError = nil

	c.logInfo("gRPC client initialized", map[string]interface{}{
		"service": c.serviceName,
	})

	return client, nil
}

// Close closes the underlying gRPC connection.
// After calling Close, the client can be reinitialized by calling GetClient again.
func (c *LazyClient[T]) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	c.logDebug("Closing gRPC client", map[string]interface{}{
		"service": c.serviceName,
	})

	err := c.conn.Close()

	// Reset state to allow reinitialization
	var zero T
	c.conn = nil
	c.client = zero
	c.initialized = false
	c.lastError = nil

	if err != nil {
		c.logError("Error closing gRPC connection", map[string]interface{}{
			"service": c.serviceName,
			"error":   err.Error(),
		})
		return fmt.Errorf("failed to close connection to %s: %w", c.serviceName, err)
	}

	return nil
}

// IsConnected returns true if the client has an active connection.
func (c *LazyClient[T]) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized && c.conn != nil && c.lastError == nil
}

// ServiceName returns the name of the service this client connects to.
func (c *LazyClient[T]) ServiceName() string {
	return c.serviceName
}

// Reset forces the client to reconnect on the next GetClient call.
// This is useful when you want to force a fresh connection (e.g., after a network issue).
func (c *LazyClient[T]) Reset() error {
	return c.Close()
}

// GetConnection returns the underlying gRPC connection, if connected.
// Returns nil if not connected. Use this for advanced use cases only.
func (c *LazyClient[T]) GetConnection() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

func (c *LazyClient[T]) logDebug(msg string, fields ...map[string]interface{}) {
	if c.log != nil {
		c.log.Debug(msg, fields...)
	} else {
		logger.Debug(msg, fields...)
	}
}

func (c *LazyClient[T]) logInfo(msg string, fields ...map[string]interface{}) {
	if c.log != nil {
		c.log.Info(msg, fields...)
	} else {
		logger.Info(msg, fields...)
	}
}

func (c *LazyClient[T]) logError(msg string, fields ...map[string]interface{}) {
	if c.log != nil {
		c.log.Error(msg, fields...)
	} else {
		logger.Error(msg, fields...)
	}
}
