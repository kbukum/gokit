package testutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/util"
)

// Implementation is one implementation of a port under contract test. New builds a fresh value for each run and registers its own cleanup with t.Cleanup.
type Implementation[T any] struct {
	Name string
	New  func(t *testing.T) T
}

// Contract runs suite once per implementation as a subtest named after it, so the in-process implementation and a remote client are held to the same behavior. Write the suite against the port's interface only.
func Contract[T any](t *testing.T, p *bootstrap.Port[T], suite func(t *testing.T, impl T), impls ...Implementation[T]) {
	t.Helper()
	if err := checkContract(suite, impls); err != nil {
		t.Fatalf("testutil: contract %s: %v", p.Name(), err)
	}
	for _, impl := range impls {
		t.Run(impl.Name, func(t *testing.T) {
			t.Helper()
			v := impl.New(t)
			if util.IsNil(v) {
				t.Fatalf("testutil: contract %s: %s returned a nil implementation", p.Name(), impl.Name)
			}
			suite(t, v)
		})
	}
}

func checkContract[T any](suite func(*testing.T, T), impls []Implementation[T]) error {
	if suite == nil {
		return errors.New("nil suite")
	}
	if len(impls) == 0 {
		return errors.New("no implementations")
	}
	seen := map[string]bool{}
	for i, impl := range impls {
		switch {
		case impl.Name == "":
			return fmt.Errorf("implementation %d has no name", i)
		case seen[impl.Name]:
			return fmt.Errorf("implementation %q listed twice", impl.Name)
		case impl.New == nil:
			return fmt.Errorf("implementation %q has no constructor", impl.Name)
		}
		seen[impl.Name] = true
	}
	return nil
}
