package testutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

type retryContainer struct {
	testcontainers.Container
	calls int
	err   error
}

func (c *retryContainer) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	c.calls++
	if c.calls == 1 {
		return c.err
	}
	return nil
}

func TestFixtureRollbackRetainsIncompleteOwnership(t *testing.T) {
	t.Parallel()
	provisionErr, cleanupErr := errors.New("provision failed"), errors.New("termination failed")
	container := &retryContainer{err: cleanupErr}
	fixture := &S3Fixture{container: container}
	owned, err := fixture.rollback(t.Context(), provisionErr)
	if owned != fixture || !errors.Is(err, provisionErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("rollback discarded ownership or causes: %v", err)
	}
	if err := owned.Close(t.Context()); err != nil || container.calls != 2 {
		t.Fatalf("cleanup retry failed: %v, calls=%d", err, container.calls)
	}
	owned, err = fixture.rollback(t.Context(), provisionErr)
	if owned != nil || !errors.Is(err, provisionErr) || container.calls != 2 {
		t.Fatalf("completed rollback retained ownership: %v", err)
	}
}

func TestFixtureRefusesInvalidOwnershipAndClosedLifecycle(t *testing.T) {
	t.Parallel()
	if f, err := StartS3(nil, t.TempDir()); f != nil || err == nil { //nolint:staticcheck // Exercise the nil-context boundary.
		t.Fatal("nil startup context accepted")
	}
	if f, err := StartS3(t.Context(), ""); f != nil || err == nil {
		t.Fatal("missing owned directory accepted")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if f, err := StartS3(t.Context(), dir); f != nil || err == nil {
		t.Fatal("existing unowned data accepted")
	}
	f := &S3Fixture{}
	for name, op := range map[string]func(context.Context) error{"restart": f.Restart, "crash restart": f.CrashRestart, "close": f.Close} {
		if err := op(nil); err == nil {
			t.Fatalf("%s accepted nil context", name)
		}
	}
	if err := f.Restart(t.Context()); err == nil {
		t.Fatal("closed fixture restarted")
	}
	if err := f.CrashRestart(t.Context()); err == nil {
		t.Fatal("closed fixture crash restarted")
	}
	if err := f.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
