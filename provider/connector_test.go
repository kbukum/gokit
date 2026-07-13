package provider_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

func TestConnector_GetClient_LazyInit(t *testing.T) {
	createCalls := 0
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "test-svc",
		Create: func() (string, error) {
			createCalls++
			return "client-instance", nil
		},
	})

	if c.IsConnected() {
		t.Error("should not be connected before GetClient")
	}
	if c.ServiceName() != "test-svc" {
		t.Errorf("ServiceName = %q, want test-svc", c.ServiceName())
	}

	client, err := c.GetClient()
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if client != "client-instance" {
		t.Errorf("client = %q, want client-instance", client)
	}
	if !c.IsConnected() {
		t.Error("should be connected after GetClient")
	}
	if createCalls != 1 {
		t.Errorf("Create called %d times, want 1", createCalls)
	}

	// Second call should reuse.
	client2, err := c.GetClient()
	if err != nil {
		t.Fatalf("second GetClient: %v", err)
	}
	if client2 != "client-instance" {
		t.Error("second call should return same client")
	}
	if createCalls != 1 {
		t.Errorf("Create called %d times after reuse, want 1", createCalls)
	}
}

func TestConnector_GetClient_ConcurrentInit(t *testing.T) {
	var createCalls atomic.Int32
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "concurrent-svc",
		Create: func() (string, error) {
			createCalls.Add(1)
			return "client", nil
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.GetClient()
			if err != nil {
				t.Errorf("GetClient: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := createCalls.Load(); n != 1 {
		t.Errorf("Create called %d times, want 1 (double-check locking failed)", n)
	}
}

func TestConnector_GetClient_CreateError(t *testing.T) {
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "failing-svc",
		Create: func() (string, error) {
			return "", errors.New("connection refused")
		},
	})

	_, err := c.GetClient()
	if err == nil {
		t.Error("expected error from failing Create")
	}
	if c.IsConnected() {
		t.Error("should not be connected after Create fails")
	}
}

func TestConnector_Close(t *testing.T) {
	closeCalled := false
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "closeable-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		OnClose: func() error {
			closeCalled = true
			return nil
		},
	})

	// Initialize first.
	_, err := c.GetClient()
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if !c.IsConnected() {
		t.Error("should be connected")
	}

	err = c.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closeCalled {
		t.Error("OnClose callback should have been called")
	}
	if c.IsConnected() {
		t.Error("should not be connected after Close")
	}
}

func TestConnector_Close_Error(t *testing.T) {
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "close-fail-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		OnClose: func() error {
			return errors.New("close failed")
		},
	})

	_, _ = c.GetClient()
	err := c.Close()
	if err == nil {
		t.Error("expected error from failing OnClose")
	}
}

func TestConnector_Reset(t *testing.T) {
	createCalls := 0
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "reset-svc",
		Create: func() (string, error) {
			createCalls++
			return "client-v" + string(rune('0'+createCalls)), nil
		},
	})

	_, _ = c.GetClient()
	if createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", createCalls)
	}

	_ = c.Reset()

	_, _ = c.GetClient()
	if createCalls != 2 {
		t.Errorf("expected 2 create calls after reset, got %d", createCalls)
	}
}

func TestConnector_Call(t *testing.T) {
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "call-svc",
		Create: func() (string, error) {
			return "my-client", nil
		},
	})

	result, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return "result-from-" + client, nil
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "result-from-my-client" {
		t.Errorf("result = %q, want result-from-my-client", result)
	}
}

func TestConnector_Call_CreateFails(t *testing.T) {
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "call-fail-svc",
		Create: func() (string, error) {
			return "", errors.New("no connection")
		},
	})

	_, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		t.Fatal("should not be called when Create fails")
		return "", nil
	})
	if err == nil {
		t.Error("expected error when Create fails")
	}
}

func TestConnector_Close_NotConnected(t *testing.T) {
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "not-connected",
		Create: func() (string, error) {
			return "client", nil
		},
		OnClose: func() error {
			t.Fatal("OnClose should not be called if never connected")
			return nil
		},
	})

	// Close without ever connecting.
	err := c.Close()
	if err != nil {
		t.Fatalf("Close on unconnected: %v", err)
	}
}

func TestConnector_WithResilienceConfig(t *testing.T) {
	t.Parallel()
	callCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "resilient-svc",
		Create: func() (string, error) {
			callCount.Add(1)
			return "client", nil
		},
		Resilience: &provider.ResilienceConfig{
			RateLimiter: &resilience.RateLimiterConfig{
				Name:  "conn-rl",
				Rate:  10000,
				Burst: 100,
			},
		},
	})

	result, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return "result-from-" + client, nil
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "result-from-client" {
		t.Fatalf("expected result-from-client, got %q", result)
	}
}
func TestConnector_ResetDuringActiveCall(t *testing.T) {
	t.Parallel()
	createCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "reset-active-svc",
		Create: func() (string, error) {
			n := createCount.Add(1)
			return fmt.Sprintf("client-v%d", n), nil
		},
	})

	// First call
	result1, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	if result1 != "client-v1" {
		t.Fatalf("expected client-v1, got %q", result1)
	}

	// Reset
	_ = c.Reset()

	// Second call should re-create
	result2, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}
	if result2 != "client-v2" {
		t.Fatalf("expected client-v2, got %q", result2)
	}
}
func TestConnector_ConcurrentCloseAndCall(t *testing.T) {
	t.Parallel()
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "concurrent-close-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		OnClose: func() error {
			return nil
		},
	})

	// Initialize
	_, _ = c.GetClient()

	var wg sync.WaitGroup
	errCount := atomic.Int32{}

	// Run Close and Call concurrently
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()
		go func() {
			defer wg.Done()
			_, err := provider.Call(context.Background(), c, func(client string) (string, error) {
				return client, nil
			})
			if err != nil {
				errCount.Add(1)
			}
		}()
	}

	wg.Wait()
	// No panics = success; some errors are expected since Close resets state
}
func TestConnector_WithCircuitBreakerResilience(t *testing.T) {
	t.Parallel()
	callCount := atomic.Int32{}
	c := provider.NewConnector(provider.ConnectorConfig[string]{
		ServiceName: "cb-conn-svc",
		Create: func() (string, error) {
			return "client", nil
		},
		Resilience: &provider.ResilienceConfig{
			CircuitBreaker: &resilience.CircuitBreakerConfig{
				Name:        "conn-cb",
				MaxFailures: 2,
				Timeout:     time.Second,
			},
		},
	})

	// Make the function fail to trip the CB
	for i := 0; i < 2; i++ {
		_, _ = provider.Call(context.Background(), c, func(client string) (string, error) {
			callCount.Add(1)
			return "", errors.New("call failed")
		})
	}

	// Next call should be rejected by CB
	_, err := provider.Call(context.Background(), c, func(client string) (string, error) {
		return "ok", nil
	})
	if err == nil {
		t.Fatal("expected circuit breaker error")
	}
	if !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}
