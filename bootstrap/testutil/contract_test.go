package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

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
