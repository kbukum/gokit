package provider_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/provider"
)

type namedRegistryWidget struct{ id int }

type namedRegistryFactory func() *namedRegistryWidget

func TestNamedRegistryRegisterGet(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	w := &namedRegistryWidget{id: 1}
	if err := r.Register("a", w); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("a")
	if !ok || got != w {
		t.Fatalf("Get(a) = %v,%v want %v,true", got, ok, w)
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatalf("Get(missing) should be !ok")
	}
}

func TestNamedRegistryRegisterEmptyName(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	err := r.Register("", &namedRegistryWidget{})
	if err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("expected empty-name error, got %v", err)
	}
}

func TestNamedRegistryRegisterNilValue(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	err := r.Register("a", nil)
	if err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("expected nil-value error, got %v", err)
	}

	rf := provider.NewNamedRegistry[namedRegistryFactory]("test")
	if err := rf.Register("a", nil); err == nil {
		t.Fatalf("nil func should error")
	}
}

func TestNamedRegistryRegisterDuplicate(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	if err := r.Register("a", &namedRegistryWidget{}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register("a", &namedRegistryWidget{})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestNamedRegistryLookup(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	if _, err := r.Lookup("missing"); err == nil {
		t.Fatalf("Lookup(missing) should error")
	}
	w := &namedRegistryWidget{id: 7}
	_ = r.Register("a", w)
	got, err := r.Lookup("a")
	if err != nil || got != w {
		t.Fatalf("Lookup(a) = %v,%v want %v,nil", got, err, w)
	}
}

func TestNamedRegistryNamesSorted(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	for _, n := range []string{"c", "a", "b"} {
		_ = r.Register(n, &namedRegistryWidget{})
	}
	got := r.Names()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Names = %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("Names[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestNamedRegistryLenEach(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	_ = r.Register("a", &namedRegistryWidget{id: 1})
	_ = r.Register("b", &namedRegistryWidget{id: 2})
	if r.Len() != 2 {
		t.Fatalf("Len = %d want 2", r.Len())
	}
	sum := 0
	r.Each(func(_ string, v *namedRegistryWidget) { sum += v.id })
	if sum != 3 {
		t.Fatalf("Each sum = %d want 3", sum)
	}
}

func TestNamedRegistryEachDoesNotHoldLockDuringCallback(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	_ = r.Register("a", &namedRegistryWidget{id: 1})

	r.Each(func(name string, _ *namedRegistryWidget) {
		if name == "a" {
			if err := r.Register("b", &namedRegistryWidget{id: 2}); err != nil {
				t.Fatalf("Register from Each callback: %v", err)
			}
		}
	})

	if r.Len() != 2 {
		t.Fatalf("Len = %d want 2", r.Len())
	}
}

func TestNamedRegistryRegisterConcurrent(t *testing.T) {
	r := provider.NewNamedRegistry[*namedRegistryWidget]("test")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.Register(string(rune('a'+(i%26))), &namedRegistryWidget{id: i})
		}(i)
	}
	wg.Wait()
	if r.Len() < 1 || r.Len() > 26 {
		t.Fatalf("unexpected Len after concurrent registers: %d", r.Len())
	}
}

func TestNamedRegistryRegisterNonNilStructValueOK(t *testing.T) {
	r := provider.NewNamedRegistry[namedRegistryWidget]("test")
	if err := r.Register("a", namedRegistryWidget{id: 1}); err != nil {
		t.Fatalf("Register struct value: %v", err)
	}
}
