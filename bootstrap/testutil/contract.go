package testutil

import (
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/kbukum/gokit/bootstrap"
	"github.com/kbukum/gokit/util"
)

// ErrNotRemoteSafe reports a port whose interface cannot be served across a process boundary.
var ErrNotRemoteSafe = errors.New("bootstrap/testutil: port is not remote-safe")

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
	// Values of these types encode themselves, so their Go shape need not be portable.
	selfEncoding = []reflect.Type{
		reflect.TypeFor[json.Marshaler](),
		reflect.TypeFor[encoding.TextMarshaler](),
		reflect.TypeFor[encoding.BinaryMarshaler](),
		reflect.TypeFor[proto.Message](),
	}
)

// RemoteSafe reports whether port p can be implemented by a remote client. Every method must take a context.Context first, so calls carry deadlines and cancellation, and return an error last, so transport failures surface. No parameter or result may be or contain a channel, function, unsafe.Pointer or interface, which cannot cross a process boundary; the check descends into pointers, slices, arrays, map keys and values and exported struct fields. The leading context.Context and trailing error are the only interfaces allowed. Concrete types implementing json.Marshaler, encoding.TextMarshaler, encoding.BinaryMarshaler or proto.Message (directly or through their pointer type) are opaque and accepted as is. Every violation is reported, each wrapping [ErrNotRemoteSafe].
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
			if j == 0 && ft.In(j) == contextType {
				continue
			}
			if problem := unportable(ft.In(j)); problem != "" {
				fail(m.Name, "parameter %d %s", j, problem)
			}
		}
		for j := range ft.NumOut() {
			if j == ft.NumOut()-1 && ft.Out(j) == errorType {
				continue
			}
			if problem := unportable(ft.Out(j)); problem != "" {
				fail(m.Name, "result %d %s", j, problem)
			}
		}
	}
	return errors.Join(errs...)
}

// unportable describes the first kind in t that cannot cross a process boundary ("is a chan", "contains a func"), or returns "" when t is portable.
func unportable(t reflect.Type) string {
	if kind := findUnportable(t, map[reflect.Type]bool{}); kind != reflect.Invalid {
		verb := "contains"
		if kind == t.Kind() && !selfEncodes(t) {
			verb = "is"
		}
		article := "a"
		if kind == reflect.Interface || kind == reflect.UnsafePointer {
			article = "an"
		}
		return fmt.Sprintf("%s %s %s", verb, article, kind)
	}
	return ""
}

func findUnportable(t reflect.Type, seen map[reflect.Type]bool) reflect.Kind {
	if seen[t] || selfEncodes(t) {
		return reflect.Invalid
	}
	seen[t] = true
	switch k := t.Kind(); k {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Interface:
		return k
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return findUnportable(t.Elem(), seen)
	case reflect.Map:
		if kind := findUnportable(t.Key(), seen); kind != reflect.Invalid {
			return kind
		}
		return findUnportable(t.Elem(), seen)
	case reflect.Struct:
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() {
				if kind := findUnportable(f.Type, seen); kind != reflect.Invalid {
					return kind
				}
			}
		}
	case reflect.Invalid, reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
	}
	return reflect.Invalid
}

func selfEncodes(t reflect.Type) bool {
	if t.Kind() == reflect.Interface {
		return false
	}
	pt := t
	if t.Kind() != reflect.Pointer {
		pt = reflect.PointerTo(t)
	}
	for _, enc := range selfEncoding {
		if t.Implements(enc) || pt.Implements(enc) {
			return true
		}
	}
	return false
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
