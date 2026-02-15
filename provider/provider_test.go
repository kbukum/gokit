package provider

import (
	"context"
	"strings"
	"testing"
)

// testProvider implements the Provider interface for testing.
type testProvider struct {
	name      string
	available bool
}

func (p *testProvider) Name() string                        { return p.name }
func (p *testProvider) IsAvailable(ctx context.Context) bool { return p.available }

func TestRegistryRegisterAndCreate(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("test", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "test", available: true}, nil
	})

	p, err := reg.Create("test", nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.Name() != "test" {
		t.Errorf("expected name 'test', got %q", p.Name())
	}
}

func TestRegistryCreateUnregistered(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	_, err := reg.Create("missing", nil)
	if err == nil {
		t.Error("expected error for unregistered factory")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' in error, got %q", err.Error())
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("beta", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "beta"}, nil
	})
	reg.RegisterFactory("alpha", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "alpha"}, nil
	})

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("expected sorted [alpha, beta], got %v", names)
	}
}

func TestRegistryGetSet(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	p := &testProvider{name: "cached", available: true}

	_, ok := reg.Get("cached")
	if ok {
		t.Error("expected Get to return false before Set")
	}

	reg.Set("cached", p)
	got, ok := reg.Get("cached")
	if !ok {
		t.Fatal("expected Get to return true after Set")
	}
	if got.Name() != "cached" {
		t.Errorf("expected 'cached', got %q", got.Name())
	}
}

func TestPrioritySelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"primary":   {name: "primary", available: false},
		"secondary": {name: "secondary", available: true},
		"tertiary":  {name: "tertiary", available: true},
	}

	sel := &PrioritySelector[*testProvider]{
		Priority: []string{"primary", "secondary", "tertiary"},
	}

	p, err := sel.Select(ctx, providers)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if p.Name() != "secondary" {
		t.Errorf("expected 'secondary' (first available), got %q", p.Name())
	}
}

func TestPrioritySelectorNoneAvailable(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
	}

	sel := &PrioritySelector[*testProvider]{Priority: []string{"a"}}
	_, err := sel.Select(ctx, providers)
	if err == nil {
		t.Error("expected error when no provider is available")
	}
}

func TestRoundRobinSelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: true},
		"b": {name: "b", available: true},
	}

	sel := &RoundRobinSelector[*testProvider]{}

	// Call multiple times to verify round-robin behavior
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		p, err := sel.Select(ctx, providers)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		seen[p.Name()]++
	}

	if len(seen) != 2 {
		t.Errorf("expected 2 different providers, got %d", len(seen))
	}
	if seen["a"] == 0 || seen["b"] == 0 {
		t.Errorf("expected both providers selected, got %v", seen)
	}
}

func TestRoundRobinSelectorEmpty(t *testing.T) {
	ctx := context.Background()
	sel := &RoundRobinSelector[*testProvider]{}
	_, err := sel.Select(ctx, map[string]*testProvider{})
	if err == nil {
		t.Error("expected error for empty providers")
	}
}

func TestHealthCheckSelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
		"b": {name: "b", available: true},
	}

	sel := &HealthCheckSelector[*testProvider]{}
	p, err := sel.Select(ctx, providers)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if p.Name() != "b" {
		t.Errorf("expected 'b' (available), got %q", p.Name())
	}
}

func TestHealthCheckSelectorNoneAvailable(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
	}

	sel := &HealthCheckSelector[*testProvider]{}
	_, err := sel.Select(ctx, providers)
	if err == nil {
		t.Error("expected error when no provider is available")
	}
}

func TestManagerInitializeAndGet(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{"main"}}
	mgr := NewManager[*testProvider](reg, sel)

	mgr.Register("main", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "main", available: true}, nil
	})

	if err := mgr.Initialize("main", nil); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	ctx := context.Background()
	p, err := mgr.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if p.Name() != "main" {
		t.Errorf("expected 'main', got %q", p.Name())
	}
}

func TestManagerGetByName(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	mgr.Register("svc", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "svc", available: true}, nil
	})
	mgr.Initialize("svc", nil)

	p, err := mgr.GetByName("svc")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if p.Name() != "svc" {
		t.Errorf("expected 'svc', got %q", p.Name())
	}
}

func TestManagerGetByNameNotFound(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	_, err := mgr.GetByName("missing")
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestManagerSetDefault(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	mgr.Register("a", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "a", available: true}, nil
	})
	mgr.Register("b", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "b", available: true}, nil
	})
	mgr.Initialize("a", nil)
	mgr.Initialize("b", nil)

	if err := mgr.SetDefault("b"); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	ctx := context.Background()
	p, err := mgr.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if p.Name() != "b" {
		t.Errorf("expected default 'b', got %q", p.Name())
	}
}

func TestManagerSetDefaultNotInitialized(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	err := mgr.SetDefault("missing")
	if err == nil {
		t.Error("expected error for setting default to uninitialized provider")
	}
}

func TestManagerAvailable(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	mgr.Register("x", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "x", available: true}, nil
	})
	mgr.Initialize("x", nil)

	avail := mgr.Available()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available, got %d", len(avail))
	}
	if avail[0] != "x" {
		t.Errorf("expected 'x', got %q", avail[0])
	}
}

func TestManagerInitializeFailure(t *testing.T) {
	reg := NewRegistry[*testProvider]()
	sel := &PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := NewManager[*testProvider](reg, sel)

	err := mgr.Initialize("unregistered", nil)
	if err == nil {
		t.Error("expected error for initializing unregistered provider")
	}
}
