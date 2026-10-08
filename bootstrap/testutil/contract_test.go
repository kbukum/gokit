package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kbukum/gokit/bootstrap"
)

type safe interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string) error
}

type unsafePort interface {
	NoContext(key string) error
	ContextLater(key string, ctx context.Context) error //nolint:revive // deliberately wrong for the test
	NoError(ctx context.Context) string
	ErrorFirst(ctx context.Context) (error, string) //nolint:revive,staticcheck // deliberately wrong for the test
	Stream(ctx context.Context, out chan<- string) error
	Callback(ctx context.Context) (func(), error)
}

func TestValidateRemoteShapeAcceptsContextAndErrorMethods(t *testing.T) {
	t.Parallel()
	if err := ValidateRemoteShape(bootstrap.NewPort[safe]("store.items")); err != nil {
		t.Fatalf("ValidateRemoteShape = %v", err)
	}
	AssertRemoteShape(t, bootstrap.NewPort[safe]("store.items"))
}

func TestValidateRemoteShapeReportsEveryViolation(t *testing.T) {
	t.Parallel()
	err := ValidateRemoteShape(bootstrap.NewPort[unsafePort]("bad.port"))
	if !errors.Is(err, ErrInvalidRemoteShape) {
		t.Fatalf("ValidateRemoteShape = %v, want ErrInvalidRemoteShape", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"bad.port NoContext: first parameter must be context.Context",
		"bad.port ContextLater: first parameter must be context.Context",
		"bad.port NoError: last result must be error",
		"bad.port ErrorFirst: last result must be error",
		"bad.port Stream: parameter 1 is a chan",
		"bad.port Callback: result 0 is a func",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not report %q:\n%s", want, msg)
		}
	}
}

func TestValidateRemoteShapeRejectsNonInterfacePort(t *testing.T) {
	t.Parallel()
	if err := ValidateRemoteShape(bootstrap.NewPort[string]("plain")); !errors.Is(err, ErrInvalidRemoteShape) {
		t.Fatalf("ValidateRemoteShape = %v", err)
	}
}

type counter interface {
	Add(ctx context.Context, n int) (int, error)
}

type localCounter struct{ total int }

func (c *localCounter) Add(_ context.Context, n int) (int, error) {
	c.total += n
	return c.total, nil
}

func TestContractRunsSuiteAgainstEveryImplementation(t *testing.T) {
	t.Parallel()
	port := bootstrap.NewPort[counter]("counter.service")
	var ran []string
	Contract(t, port, func(t *testing.T, c counter) {
		ran = append(ran, t.Name())
		if got, err := c.Add(context.Background(), 2); err != nil || got != 2 {
			t.Fatalf("Add = %d, %v", got, err)
		}
	},
		Implementation[counter]{Name: "local", New: func(*testing.T) counter { return &localCounter{} }},
		Implementation[counter]{Name: "other", New: func(*testing.T) counter { return &localCounter{} }},
	)
	want := []string{t.Name() + "/local", t.Name() + "/other"}
	if strings.Join(ran, ",") != strings.Join(want, ",") {
		t.Fatalf("ran %v, want %v", ran, want)
	}
}

func TestCheckContractRejectsInvalidSetup(t *testing.T) {
	t.Parallel()
	newCounter := func(*testing.T) counter { return &localCounter{} }
	suite := func(*testing.T, counter) {}
	for name, tc := range map[string]struct {
		suite func(*testing.T, counter)
		impls []Implementation[counter]
	}{
		"no implementations": {suite, nil},
		"nil suite":          {nil, []Implementation[counter]{{Name: "a", New: newCounter}}},
		"unnamed":            {suite, []Implementation[counter]{{New: newCounter}}},
		"duplicate name":     {suite, []Implementation[counter]{{Name: "a", New: newCounter}, {Name: "a", New: newCounter}}},
		"nil constructor":    {suite, []Implementation[counter]{{Name: "a"}}},
	} {
		if err := checkContract(tc.suite, tc.impls); err == nil {
			t.Errorf("%s: checkContract succeeded", name)
		}
	}
	if err := checkContract(suite, []Implementation[counter]{{Name: "a", New: newCounter}}); err != nil {
		t.Fatalf("valid setup = %v", err)
	}
}

type nestedRequest struct {
	Name     string
	Callback func() `json:"-"`
	hidden   chan int
}

type nestedOK struct {
	Items  []string
	ByName map[string]*nestedOK // recursive
	At     time.Time
}

type embeddedValue struct{ nestedRequest }

type embeddedPointer struct{ *nestedRequest }

type embeddedSafe struct{ *nestedOK }

type hiddenCallback func()

type hiddenScalar struct{ hiddenCallback }

type embeddedPort interface {
	Value(ctx context.Context, in embeddedValue) error
	Pointer(ctx context.Context) (embeddedPointer, error)
	Safe(ctx context.Context, in embeddedSafe) error
	Hidden(ctx context.Context, in hiddenScalar) error
}

func TestValidateRemoteShapeInspectsUnexportedEmbeddedStructs(t *testing.T) {
	t.Parallel()
	err := ValidateRemoteShape(bootstrap.NewPort[embeddedPort]("embedded"))
	if !errors.Is(err, ErrInvalidRemoteShape) {
		t.Fatalf("ValidateRemoteShape = %v, want ErrInvalidRemoteShape", err)
	}
	for _, want := range []string{
		"embedded Value: parameter 1 contains a func",
		"embedded Pointer: result 0 contains a func",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateRemoteShape = %v, missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "Safe") || strings.Contains(err.Error(), "Hidden") {
		t.Errorf("ValidateRemoteShape rejected safe or unexported scalar fields: %v", err)
	}
}

// selfEncoded hides a func behind its own encoding, so ValidateRemoteShape treats it as opaque.
type selfEncoded struct{ Hook func() }

func (selfEncoded) MarshalText() ([]byte, error) { return []byte("x"), nil }

type nestedPort interface {
	Slice(ctx context.Context, in []chan string) error
	Map(ctx context.Context, in map[string]func()) error
	Struct(ctx context.Context, in *nestedRequest) error
	Any(ctx context.Context, in any) error
	Wrapped(ctx context.Context, in []any) error
	Encoded(ctx context.Context, in selfEncoded) error
	Result(ctx context.Context) ([]nestedRequest, error)
	Complex(ctx context.Context, in complex128) error
	ComplexField(ctx context.Context, in []struct{ Value complex64 }) error
	Fine(ctx context.Context, in nestedOK) (*nestedOK, error)
}

type notProtobuf struct{ Callback func() }

func (*notProtobuf) ProtoReflect() {}

type protobufPort interface {
	Message(ctx context.Context, in *structpb.Struct) (*structpb.Value, error)
}

type notProtobufPort interface {
	Message(ctx context.Context, in *notProtobuf) error
}

func TestValidateRemoteShapeRequiresTheProtobufMessageContract(t *testing.T) {
	t.Parallel()
	if err := ValidateRemoteShape(bootstrap.NewPort[protobufPort]("protobuf")); err != nil {
		t.Errorf("protobuf message rejected: %v", err)
	}
	err := ValidateRemoteShape(bootstrap.NewPort[notProtobufPort]("not-protobuf"))
	if !errors.Is(err, ErrInvalidRemoteShape) || !strings.Contains(err.Error(), "contains a func") {
		t.Errorf("ValidateRemoteShape = %v, want rejection of a non-protobuf callback field", err)
	}
}

func TestValidateRemoteShapeInspectsNestedTypes(t *testing.T) {
	t.Parallel()
	err := ValidateRemoteShape(bootstrap.NewPort[nestedPort]("nested"))
	if !errors.Is(err, ErrInvalidRemoteShape) {
		t.Fatalf("ValidateRemoteShape = %v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"nested Slice: parameter 1 contains a chan",
		"nested Map: parameter 1 contains a func",
		"nested Struct: parameter 1 contains a func",
		"nested Any: parameter 1 is an interface",
		"nested Wrapped: parameter 1 contains an interface",
		"nested Result: result 0 contains a func",
		"nested Complex: parameter 1 is a complex128",
		"nested ComplexField: parameter 1 contains a complex64",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not report %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Fine") || strings.Contains(msg, "Encoded") || strings.Contains(msg, "hidden") {
		t.Errorf("reported a safe method or an unexported field:\n%s", msg)
	}
}
