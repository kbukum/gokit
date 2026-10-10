package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestRunPortTaskCallsTheProvidedPort(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustUse(t, app, ValueModule("store-double", storePort, aStore()))
	var got string
	err := RunPortTask(context.Background(), app, storePort, func(_ context.Context, s store) error {
		got = s.Get("k")
		return nil
	})
	if err != nil || got != "value:k" {
		t.Fatalf("task saw %q, %v", got, err)
	}
}

func TestRunPortTaskReportsMissingProviderAndTaskFailure(t *testing.T) {
	t.Parallel()
	ran := false
	err := RunPortTask(context.Background(), newQuietApp(t), storePort, func(context.Context, store) error {
		ran = true
		return nil
	})
	var moduleErr *ModuleError
	if !errors.As(err, &moduleErr) || ran {
		t.Fatalf("missing provider: ran=%t err=%v", ran, err)
	}

	app := newQuietApp(t)
	mustUse(t, app, ValueModule("store-double", storePort, aStore()))
	boom := errors.New("boom")
	err = RunPortTask(context.Background(), app, storePort, func(context.Context, store) error { return boom })
	var taskErr *TaskError
	if !errors.As(err, &taskErr) || !errors.Is(err, boom) {
		t.Fatalf("task failure: %v", err)
	}
}

func TestRunPortTaskRejectsUseAfterLifecycle(t *testing.T) {
	t.Parallel()
	app := newQuietApp(t)
	mustUse(t, app, ValueModule("store-double", storePort, aStore()))
	noop := func(context.Context, store) error { return nil }
	if err := RunPortTask(context.Background(), app, storePort, noop); err != nil {
		t.Fatal(err)
	}

	if err := RunPortTask(context.Background(), app, storePort, noop); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("second run: %v", err)
	}
	if err := RunPortTask[*testConfig, store](context.Background(), app, storePort, nil); err == nil {
		t.Fatal("nil task accepted")
	}
}

func TestRunPortTaskRejectsNilDependencies(t *testing.T) {
	t.Parallel()
	noop := func(context.Context, store) error { return nil }
	if err := RunPortTask[*testConfig, store](context.Background(), nil, storePort, noop); err == nil {
		t.Fatal("nil app accepted")
	}
	if err := RunPortTask(context.Background(), newQuietApp(t), (*Port[store])(nil), noop); err == nil {
		t.Fatal("nil port accepted")
	}
}
