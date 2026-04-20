package provider

import (
	"fmt"
	"strings"
	"testing"
)

func TestOperationRegistry_Resolve_SingleBinding(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("openai", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "openai", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "transcribe",
		ProviderName: "openai",
		Priority:     1,
	})

	p, err := opReg.Resolve("transcribe", "free")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected openai, got %q", p.Name())
	}
}

func TestOperationRegistry_Resolve_PriorityOrder(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("expensive", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "expensive", available: true}, nil
	})
	reg.RegisterFactory("cheap", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "cheap", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "translate",
		ProviderName: "expensive",
		Priority:     10,
	})
	opReg.Bind(OperationBinding{
		OperationID:  "translate",
		ProviderName: "cheap",
		Priority:     1,
	})

	p, err := opReg.Resolve("translate", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name() != "cheap" {
		t.Errorf("expected cheap (lower priority), got %q", p.Name())
	}
}

func TestOperationRegistry_Resolve_TierFiltering(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("premium-provider", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "premium-provider", available: true}, nil
	})
	reg.RegisterFactory("basic-provider", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "basic-provider", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "analyze",
		ProviderName: "premium-provider",
		Tiers:        []string{"pro", "enterprise"},
		Priority:     1,
	})
	opReg.Bind(OperationBinding{
		OperationID:  "analyze",
		ProviderName: "basic-provider",
		Priority:     5,
	})

	// Pro tier should get premium-provider.
	p, err := opReg.Resolve("analyze", "pro")
	if err != nil {
		t.Fatalf("Resolve pro: %v", err)
	}
	if p.Name() != "premium-provider" {
		t.Errorf("expected premium-provider for pro tier, got %q", p.Name())
	}

	// Free tier should only get basic-provider.
	// Clear cached instance so the factory is called again for basic-provider.
	reg2 := NewRegistry[*testProvider]()
	reg2.RegisterFactory("premium-provider", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "premium-provider", available: true}, nil
	})
	reg2.RegisterFactory("basic-provider", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "basic-provider", available: true}, nil
	})
	opReg2 := NewOperationRegistry(reg2)
	opReg2.Bind(OperationBinding{
		OperationID:  "analyze",
		ProviderName: "premium-provider",
		Tiers:        []string{"pro", "enterprise"},
		Priority:     1,
	})
	opReg2.Bind(OperationBinding{
		OperationID:  "analyze",
		ProviderName: "basic-provider",
		Priority:     5,
	})

	p2, err := opReg2.Resolve("analyze", "free")
	if err != nil {
		t.Fatalf("Resolve free: %v", err)
	}
	if p2.Name() != "basic-provider" {
		t.Errorf("expected basic-provider for free tier, got %q", p2.Name())
	}
}

func TestOperationRegistry_Resolve_NoBindings(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	opReg := NewOperationRegistry(reg)

	_, err := opReg.Resolve("nonexistent", "free")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
	if !strings.Contains(err.Error(), "no bindings") {
		t.Errorf("expected 'no bindings' in error, got %q", err.Error())
	}
}

func TestOperationRegistry_Resolve_NoTierAccess(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("restricted", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "restricted", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "secret-op",
		ProviderName: "restricted",
		Tiers:        []string{"admin"},
		Priority:     1,
	})

	_, err := opReg.Resolve("secret-op", "free")
	if err == nil {
		t.Fatal("expected error for inaccessible tier")
	}
	if !strings.Contains(err.Error(), "tier") {
		t.Errorf("expected 'tier' in error, got %q", err.Error())
	}
}

func TestOperationRegistry_Resolve_FallbackOnFactoryError(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("broken", func(cfg map[string]any) (*testProvider, error) {
		return nil, fmt.Errorf("init failed")
	})
	reg.RegisterFactory("working", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "working", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "op",
		ProviderName: "broken",
		Priority:     1,
	})
	opReg.Bind(OperationBinding{
		OperationID:  "op",
		ProviderName: "working",
		Priority:     2,
	})

	p, err := opReg.Resolve("op", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Name() != "working" {
		t.Errorf("expected fallback to working, got %q", p.Name())
	}
}

func TestOperationRegistry_Resolve_UsesCachedInstance(t *testing.T) {
	t.Parallel()

	callCount := 0
	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("counted", func(cfg map[string]any) (*testProvider, error) {
		callCount++
		return &testProvider{name: "counted", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "op",
		ProviderName: "counted",
		Priority:     1,
	})

	// First resolve creates the instance.
	_, err := opReg.Resolve("op", "")
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected factory called once, got %d", callCount)
	}

	// Second resolve should use cached instance.
	_, err = opReg.Resolve("op", "")
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected factory still called once (cached), got %d", callCount)
	}
}

func TestOperationRegistry_ListBindings(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{OperationID: "op", ProviderName: "b", Priority: 5})
	opReg.Bind(OperationBinding{OperationID: "op", ProviderName: "a", Priority: 1})
	opReg.Bind(OperationBinding{OperationID: "other", ProviderName: "c", Priority: 3})

	bindings := opReg.ListBindings("op")
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	if bindings[0].ProviderName != "a" || bindings[1].ProviderName != "b" {
		t.Errorf("expected sorted by priority [a, b], got [%s, %s]",
			bindings[0].ProviderName, bindings[1].ProviderName)
	}
}

func TestOperationRegistry_ListBindings_Empty(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	opReg := NewOperationRegistry(reg)

	bindings := opReg.ListBindings("nonexistent")
	if len(bindings) != 0 {
		t.Errorf("expected 0 bindings, got %d", len(bindings))
	}
}

func TestOperationRegistry_EmptyTiersMeansAll(t *testing.T) {
	t.Parallel()

	reg := NewRegistry[*testProvider]()
	reg.RegisterFactory("universal", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "universal", available: true}, nil
	})

	opReg := NewOperationRegistry(reg)
	opReg.Bind(OperationBinding{
		OperationID:  "op",
		ProviderName: "universal",
		Tiers:        nil,
		Priority:     1,
	})

	for _, tier := range []string{"free", "pro", "enterprise", ""} {
		p, err := opReg.Resolve("op", tier)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tier, err)
		}
		if p.Name() != "universal" {
			t.Errorf("Resolve(%q) = %q, want universal", tier, p.Name())
		}
	}
}

// compile-time check: testProvider (from provider_test.go) satisfies Provider.
var _ Provider = (*testProvider)(nil)

func TestTierAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tiers []string
		tier  string
		want  bool
	}{
		{nil, "any", true},
		{[]string{}, "any", true},
		{[]string{"pro"}, "pro", true},
		{[]string{"pro"}, "free", false},
		{[]string{"pro", "enterprise"}, "enterprise", true},
		{[]string{"pro", "enterprise"}, "free", false},
	}

	for _, tc := range tests {
		got := tierAllowed(tc.tiers, tc.tier)
		if got != tc.want {
			t.Errorf("tierAllowed(%v, %q) = %v, want %v", tc.tiers, tc.tier, got, tc.want)
		}
	}
}

// compile-time check: testProvider (from provider_test.go) satisfies Provider.
var _ Provider = (*testProvider)(nil)
