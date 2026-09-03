package workload_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
)

// stubRuntime is a minimal ModelRuntime used to exercise registry wiring.
type stubRuntime struct {
	cfg workload.ModelRuntimeConfig
}

func (s *stubRuntime) Start(_ context.Context, spec workload.ModelSpec) (*workload.ModelHandle, error) {
	return &workload.ModelHandle{Model: spec.Ref, Status: workload.StatusRunning}, nil
}
func (s *stubRuntime) Stop(context.Context, string) error { return nil }
func (s *stubRuntime) Health(context.Context) error       { return nil }
func (s *stubRuntime) Endpoint(_ context.Context, model string) (*workload.Endpoint, error) {
	return &workload.Endpoint{BaseURL: "http://stub", Model: model, API: workload.APIOpenAI}, nil
}

func (s *stubRuntime) Stats(context.Context, string) (*workload.ModelStats, error) {
	return &workload.ModelStats{}, nil
}

func newStubRegistry(t *testing.T, name string) *workload.ModelRuntimeRegistry {
	t.Helper()
	reg := workload.NewModelRuntimeRegistry()
	err := reg.Register(name, func(cfg workload.ModelRuntimeConfig, _ *logging.Logger) (workload.ModelRuntime, error) {
		return &stubRuntime{cfg: cfg}, nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return reg
}

func TestNewModelRuntimeSelectsProvider(t *testing.T) {
	t.Parallel()
	reg := newStubRegistry(t, "stub")

	rt, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: "stub"}, testLogger())
	if err != nil {
		t.Fatalf("NewModelRuntime: %v", err)
	}
	h, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Model != "ai/smollm2" || h.Status != workload.StatusRunning {
		t.Fatalf("unexpected handle: %+v", h)
	}
}

func TestNewModelRuntimeEmptyProviderInvalid(t *testing.T) {
	t.Parallel()
	reg := newStubRegistry(t, workload.ProviderDMR)

	// The port keeps no built-in runtime, so an empty provider is invalid and
	// must not silently default to any backend.
	if _, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{}, testLogger()); err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestNewModelRuntimeUnknownProvider(t *testing.T) {
	t.Parallel()
	reg := newStubRegistry(t, "stub")

	_, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: "nope"}, testLogger())
	if err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}

func TestNewModelRuntimeNilRegistry(t *testing.T) {
	t.Parallel()
	_, err := workload.NewModelRuntime(nil, workload.ModelRuntimeConfig{Provider: "stub"}, testLogger())
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestNewModelRuntimeNilLoggerDefaults(t *testing.T) {
	t.Parallel()
	reg := newStubRegistry(t, "stub")
	// A nil logger must be defaulted rather than panic.
	if _, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: "stub"}, nil); err != nil {
		t.Fatalf("NewModelRuntime with nil logger: %v", err)
	}
}

func TestModelRuntimeRegistryDuplicate(t *testing.T) {
	t.Parallel()
	reg := newStubRegistry(t, "stub")
	err := reg.Register("stub", func(workload.ModelRuntimeConfig, *logging.Logger) (workload.ModelRuntime, error) {
		return nil, errors.New("unused")
	})
	if err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func testLogger() *logging.Logger {
	return logging.MustNew(&logging.Config{Level: "error", Format: "json"}, "modelruntime-test")
}
