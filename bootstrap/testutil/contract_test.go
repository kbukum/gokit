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

func TestRemoteSafeAcceptsContextAndErrorMethods(t *testing.T) {
	t.Parallel()
	if err := RemoteSafe(bootstrap.NewPort[safe]("store.items")); err != nil {
		t.Fatalf("RemoteSafe = %v", err)
	}
	AssertRemoteSafe(t, bootstrap.NewPort[safe]("store.items"))
}

func TestRemoteSafeReportsEveryViolation(t *testing.T) {
	t.Parallel()
	err := RemoteSafe(bootstrap.NewPort[unsafePort]("bad.port"))
	if !errors.Is(err, ErrNotRemoteSafe) {
		t.Fatalf("RemoteSafe = %v, want ErrNotRemoteSafe", err)
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

func TestRemoteSafeRejectsNonInterfacePort(t *testing.T) {
	t.Parallel()
	if err := RemoteSafe(bootstrap.NewPort[string]("plain")); !errors.Is(err, ErrNotRemoteSafe) {
		t.Fatalf("RemoteSafe = %v", err)
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
		Impl[counter]{Name: "local", New: func(*testing.T) counter { return &localCounter{} }},
		Impl[counter]{Name: "other", New: func(*testing.T) counter { return &localCounter{} }},
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
		impls []Impl[counter]
	}{
		"no implementations": {suite, nil},
		"nil suite":          {nil, []Impl[counter]{{Name: "a", New: newCounter}}},
		"unnamed":            {suite, []Impl[counter]{{New: newCounter}}},
		"duplicate name":     {suite, []Impl[counter]{{Name: "a", New: newCounter}, {Name: "a", New: newCounter}}},
		"nil constructor":    {suite, []Impl[counter]{{Name: "a"}}},
	} {
		if err := checkContract(tc.suite, tc.impls); err == nil {
			t.Errorf("%s: checkContract succeeded", name)
		}
	}
	if err := checkContract(suite, []Impl[counter]{{Name: "a", New: newCounter}}); err != nil {
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

// selfEncoded hides a func behind its own encoding, so RemoteSafe treats it as opaque.
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

func TestRemoteSafeRequiresTheProtobufMessageContract(t *testing.T) {
	t.Parallel()
	if err := RemoteSafe(bootstrap.NewPort[protobufPort]("protobuf")); err != nil {
		t.Errorf("protobuf message rejected: %v", err)
	}
	err := RemoteSafe(bootstrap.NewPort[notProtobufPort]("not-protobuf"))
	if !errors.Is(err, ErrNotRemoteSafe) || !strings.Contains(err.Error(), "contains a func") {
		t.Errorf("RemoteSafe = %v, want rejection of a non-protobuf callback field", err)
	}
}

func TestRemoteSafeInspectsNestedTypes(t *testing.T) {
	t.Parallel()
	err := RemoteSafe(bootstrap.NewPort[nestedPort]("nested"))
	if !errors.Is(err, ErrNotRemoteSafe) {
		t.Fatalf("RemoteSafe = %v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"nested Slice: parameter 1 contains a chan",
		"nested Map: parameter 1 contains a func",
		"nested Struct: parameter 1 contains a func",
		"nested Any: parameter 1 is an interface",
		"nested Wrapped: parameter 1 contains an interface",
		"nested Result: result 0 contains a func",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not report %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Fine") || strings.Contains(msg, "Encoded") || strings.Contains(msg, "hidden") {
		t.Errorf("reported a safe method or an unexported field:\n%s", msg)
	}
}
