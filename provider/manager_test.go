package provider_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

func TestManager_WithLoggerAndDefault(t *testing.T) {
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector,
		provider.WithLogger[provider.RequestResponse[string, string]](slog.Default()),
	)

	registry.RegisterFactory("echo", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &echoProvider{name: "echo"}, nil
	})
	if err := mgr.InitializeWithContext(context.Background(), "echo", nil); err != nil {
		t.Fatalf("initialize error: %v", err)
	}

	mgr.SetDefault("echo")
	p, err := mgr.Get(context.Background())
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if p.Name() != "echo" {
		t.Fatalf("expected echo provider, got %q", p.Name())
	}
}

func TestManager_InitializeWithResilience_FactoryError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("fail-factory", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return nil, errors.New("factory error")
	})

	err := mgr.InitializeWithResilience(context.Background(), "fail-factory", nil, func(p provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		t.Fatal("wrapper should not be called when factory fails")
		return p
	})
	if err == nil {
		t.Fatal("expected factory error")
	}
	if !strings.Contains(err.Error(), "factory error") {
		t.Fatalf("expected factory error in message, got %q", err.Error())
	}
}

func TestManager_InitializeWithContext_CancellationMidInit(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	ctx, cancel := context.WithCancel(context.Background())

	registry.RegisterFactory("slow-init", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &slowInitProvider{name: "slow", initDelay: 500 * time.Millisecond}, nil
	})

	// Cancel context during init
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := mgr.InitializeWithContext(ctx, "slow-init", nil)
	if err == nil {
		t.Fatal("expected error from canceled context during init")
	}
}

func TestManager_CloseAll_MixedCloseableNonCloseable(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	closeCalled := false
	registry.RegisterFactory("closeable", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &closeTrackingProvider{name: "closeable", closeCalled: &closeCalled}, nil
	})
	registry.RegisterFactory("non-closeable", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &nonCloseableProvider{name: "non-closeable"}, nil
	})

	mgr.Initialize("closeable", nil)
	mgr.Initialize("non-closeable", nil)

	err := mgr.CloseAll(context.Background())
	if err != nil {
		t.Fatalf("CloseAll should succeed: %v", err)
	}
	if !closeCalled {
		t.Fatal("expected Close to be called on closeable provider")
	}
}

func TestManager_SetDefault_NonExistent(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	err := mgr.SetDefault("nonexistent")
	if err == nil {
		t.Fatal("expected error setting default to non-existent provider")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected 'not initialized' error, got %q", err.Error())
	}
}

func TestManager_Get_NoDefaultNoProviders(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	_, err := mgr.Get(context.Background())
	if err == nil {
		t.Fatal("expected error when no providers exist")
	}
}

func TestManager_CloseAll_WithCloseError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("close-err", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &closeErrProvider{name: "close-err"}, nil
	})

	mgr.Initialize("close-err", nil)

	err := mgr.CloseAll(context.Background())
	if err == nil {
		t.Fatal("expected error from CloseAll when a provider Close fails")
	}
	if !strings.Contains(err.Error(), "close error") {
		t.Fatalf("expected 'close error' in message, got %q", err.Error())
	}
}

func TestManager_InitializeWithResilience_InitError(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.HealthCheckSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	registry.RegisterFactory("init-err", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return &initErrProvider{name: "init-err"}, nil
	})

	err := mgr.InitializeWithResilience(context.Background(), "init-err", nil, func(p provider.RequestResponse[string, string]) provider.RequestResponse[string, string] {
		t.Fatal("wrapper should not be called when init fails")
		return p
	})
	if err == nil {
		t.Fatal("expected init error")
	}
	if !strings.Contains(err.Error(), "init failed") {
		t.Fatalf("expected 'init failed' error, got %q", err.Error())
	}
}

func TestManager_ConcurrentGetAndInitialize(t *testing.T) {
	t.Parallel()
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	selector := &provider.RoundRobinSelector[provider.RequestResponse[string, string]]{}
	mgr := provider.NewManager(registry, selector)

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("p%d", i)
		registry.RegisterFactory(name, func(_ map[string]any) (provider.RequestResponse[string, string], error) {
			return &echoProvider{name: name}, nil
		})
	}

	var wg sync.WaitGroup

	// Initialize concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = mgr.Initialize(fmt.Sprintf("p%d", i), nil)
		}(i)
	}
	wg.Wait()

	// Get concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.Get(context.Background())
		}()
	}
	wg.Wait()

	if len(mgr.Available()) != 5 {
		t.Fatalf("expected 5 providers, got %d", len(mgr.Available()))
	}
}
