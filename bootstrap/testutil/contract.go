package testutil

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/util"
)

// ErrNotRemoteSafe reports a port whose interface cannot be served across a process boundary.
var ErrNotRemoteSafe = errors.New("bootstrap/testutil: port is not remote-safe")

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
)

// RemoteSafe reports whether port p can be implemented by a remote client. Every method must take a context.Context first, so calls carry deadlines and cancellation, and return an error last, so transport failures surface. No parameter or result may be a channel, function or unsafe.Pointer, which cannot cross a process boundary. Every violation is reported, each wrapping [ErrNotRemoteSafe].
//
// The check covers shape only. Whether values are safe to copy, calls tolerate latency and errors map to the same kinds remotely is what [Contract] tests.
func RemoteSafe[T any](p *bootstrap.Port[T]) error {
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Interface {
		return fmt.Errorf("%w: %s: %s is not an interface", ErrNotRemoteSafe, p.Name(), typ)
	}
	var errs []error
	fail := func(method, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: %s %s: %s", ErrNotRemoteSafe, p.Name(), method, fmt.Sprintf(format, args...)))
	}
	for i := range typ.NumMethod() {
		m := typ.Method(i)
		ft := m.Type
		if ft.NumIn() == 0 || ft.In(0) != contextType {
			fail(m.Name, "first parameter must be context.Context")
		}
		if ft.NumOut() == 0 || ft.Out(ft.NumOut()-1) != errorType {
			fail(m.Name, "last result must be error")
		}
		for j := range ft.NumIn() {
			if kind, bad := unportable(ft.In(j)); bad {
				fail(m.Name, "parameter %d is a %s", j, kind)
			}
		}
		for j := range ft.NumOut() {
			if kind, bad := unportable(ft.Out(j)); bad {
				fail(m.Name, "result %d is a %s", j, kind)
			}
		}
	}
	return errors.Join(errs...)
}

func unportable(t reflect.Type) (reflect.Kind, bool) {
	switch k := t.Kind(); k {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return k, true
	default:
		return k, false
	}
}

// AssertRemoteSafe fails t when [RemoteSafe] reports a violation.
func AssertRemoteSafe[T any](t testing.TB, p *bootstrap.Port[T]) {
	t.Helper()
	if err := RemoteSafe(p); err != nil {
		t.Fatal(err)
	}
}

// Impl is one implementation of a port under contract test. New builds a fresh value for each run and registers its own cleanup with t.Cleanup.
type Impl[T any] struct {
	Name string
	New  func(t *testing.T) T
}

// Contract runs suite once per implementation as a subtest named after it, so the in-process implementation and a remote client are held to the same behavior. Write the suite against the port's interface only.
func Contract[T any](t *testing.T, p *bootstrap.Port[T], suite func(t *testing.T, impl T), impls ...Impl[T]) {
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

func checkContract[T any](suite func(*testing.T, T), impls []Impl[T]) error {
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
