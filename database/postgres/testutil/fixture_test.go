package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	componenttest "github.com/kbukum/gokit/component/testutil"
)

func TestFixtureCleanupUsesFreshBudgetAndRetriesFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("termination failed")
	calls := 0
	double := &componenttest.Component{StopFunc: func(ctx context.Context) error {
		calls++
		if calls > 2 {
			return nil
		}
		if ctx.Err() != nil {
			t.Errorf("canceled cleanup: %v", ctx.Err())
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Error("missing termination budget")
		}
		return failure
	}}
	fixture := &Fixture{terminate: double.Stop}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for range 2 {
		if err := fixture.Close(ctx); !errors.Is(err, failure) {
			t.Fatalf("termination failure hidden: %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("failed termination was not retried: calls = %d", calls)
	}
	for range 2 {
		if err := fixture.Close(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 {
		t.Fatalf("successful termination repeated: calls = %d", calls)
	}
	if _, err := fixture.NewDatabase(t.Context()); err == nil {
		t.Fatal("closed fixture created a database")
	}
}

func TestDockerPrerequisiteFailureDoesNotSkip(t *testing.T) {
	t.Setenv("DOCKER_HOST", "://invalid")
	if err := DockerReady(t.Context()); err == nil {
		t.Fatal("invalid Docker prerequisite certified success")
	}
}

func TestCanceledProvisioningReturnsSetupFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fixture, err := Start(ctx)
	if err == nil || fixture != nil {
		t.Fatalf("canceled provisioning: %v %v", fixture, err)
	}
}
