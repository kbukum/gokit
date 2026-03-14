package provider

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConnector_GetClient_LazyInit(t *testing.T) {
	createCalls := 0
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
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
	c := NewConnector(ConnectorConfig[string]{
		ServiceName: "call-svc",
		Create: func() (string, error) {
			return "my-client", nil
		},
	})

	result, err := Call(context.Background(), c, func(client string) (string, error) {
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
	c := NewConnector(ConnectorConfig[string]{
		ServiceName: "call-fail-svc",
		Create: func() (string, error) {
			return "", errors.New("no connection")
		},
	})

	_, err := Call(context.Background(), c, func(client string) (string, error) {
		t.Fatal("should not be called when Create fails")
		return "", nil
	})
	if err == nil {
		t.Error("expected error when Create fails")
	}
}

func TestConnector_Close_NotConnected(t *testing.T) {
	c := NewConnector(ConnectorConfig[string]{
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
