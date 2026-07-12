package di_test

import (
	"context"
	"sort"
	"testing"

	"github.com/kbukum/gokit/di"
)

func TestRegistrations_ReportsModeAndInitialization(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, 1, di.WithName("eager"))
	_ = di.RegisterSingleton(c, func(context.Context) (string, error) { return "s", nil }, di.WithName("singleton"))
	_ = di.RegisterTransient(c, func(context.Context) (float64, error) { return 1.5, nil }, di.WithName("transient"))

	regs := c.Registrations()
	sort.Slice(regs, func(i, j int) bool { return regs[i].Key < regs[j].Key })
	if len(regs) != 3 {
		t.Fatalf("expected 3 registrations, got %d", len(regs))
	}

	byKey := map[string]di.RegistrationInfo{}
	for _, r := range regs {
		byKey[r.Key] = r
	}

	if r := byKey["eager"]; r.Mode != di.Eager || !r.Initialized {
		t.Errorf("eager: mode=%v init=%v", r.Mode, r.Initialized)
	}
	if r := byKey["singleton"]; r.Mode != di.Singleton || r.Initialized {
		t.Errorf("singleton before resolve: mode=%v init=%v", r.Mode, r.Initialized)
	}
	if r := byKey["transient"]; r.Mode != di.Transient || r.Initialized {
		t.Errorf("transient: mode=%v init=%v", r.Mode, r.Initialized)
	}
}

func TestRegistrations_SingletonInitializedAfterResolve(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.RegisterSingleton(c, func(context.Context) (string, error) { return "s", nil })
	_, _ = di.Resolve[string](context.Background(), c)

	regs := c.Registrations()
	if len(regs) != 1 || !regs[0].Initialized {
		t.Fatalf("expected initialized singleton, got %+v", regs)
	}
	if regs[0].Type != "string" {
		t.Errorf("expected Type 'string', got %q", regs[0].Type)
	}
}

func TestRegistrations_KeyIsTypeWhenUnnamed(t *testing.T) {
	t.Parallel()
	c := di.NewContainer()
	_ = di.Register(c, 42)
	regs := c.Registrations()
	if len(regs) != 1 || regs[0].Key != "int" {
		t.Fatalf("expected key 'int', got %+v", regs)
	}
}

func TestMode_String(t *testing.T) {
	t.Parallel()
	cases := map[di.Mode]string{
		di.Eager:     "eager",
		di.Singleton: "singleton",
		di.Transient: "transient",
		di.Mode(99):  "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("Mode(%d).String() = %q, want %q", m, got, want)
		}
	}
}
