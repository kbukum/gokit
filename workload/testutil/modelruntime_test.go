package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/workload"
)

func TestMockModelRuntimeLifecycle(t *testing.T) {
	t.Parallel()
	rt := NewMockModelRuntime()
	ctx := context.Background()

	if err := rt.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}

	h, err := rt.Start(ctx, workload.ModelSpec{Ref: "ai/smollm2"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Model != "ai/smollm2" || h.Status != workload.StatusRunning {
		t.Fatalf("unexpected handle: %+v", h)
	}
	if h.Endpoint.Model != "ai/smollm2" || h.Endpoint.API != workload.APIOpenAI {
		t.Fatalf("unexpected endpoint: %+v", h.Endpoint)
	}

	ep, err := rt.Endpoint(ctx, "ai/smollm2")
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if ep.BaseURL == "" {
		t.Fatal("expected non-empty base URL")
	}

	stats, err := rt.Stats(ctx, "ai/smollm2")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if !stats.Loaded {
		t.Fatal("expected running model to report Loaded")
	}

	models, err := rt.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	if err := rt.Stop(ctx, "ai/smollm2"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stats, _ = rt.Stats(ctx, "ai/smollm2")
	if stats.Loaded {
		t.Fatal("expected stopped model to report not Loaded")
	}

	if err := rt.RemoveModel(ctx, "ai/smollm2"); err != nil {
		t.Fatalf("RemoveModel: %v", err)
	}
	models, _ = rt.ListModels(ctx)
	if len(models) != 0 {
		t.Fatalf("expected 0 models after remove, got %d", len(models))
	}
}

func TestMockModelRuntimeIdempotentStopAndErrors(t *testing.T) {
	t.Parallel()
	rt := NewMockModelRuntime()
	ctx := context.Background()

	// Stop of an unknown model is a no-op.
	if err := rt.Stop(ctx, "missing"); err != nil {
		t.Fatalf("Stop unknown: %v", err)
	}
	// Endpoint is a pure address computation: it succeeds for any non-empty
	// model (mirroring the real backends) and errors only on empty input.
	if _, err := rt.Endpoint(ctx, "missing"); err != nil {
		t.Fatalf("Endpoint for unstarted model should succeed: %v", err)
	}
	if _, err := rt.Endpoint(ctx, ""); err == nil {
		t.Fatal("expected Endpoint error for empty model")
	}
	// Stats and Remove of an unknown model error.
	if _, err := rt.Stats(ctx, "missing"); err == nil {
		t.Fatal("expected Stats error for unknown model")
	}
	if err := rt.RemoveModel(ctx, "missing"); err == nil {
		t.Fatal("expected RemoveModel error for unknown model")
	}

	// Injected health failure surfaces.
	want := fmt.Errorf("boom")
	rt.SetHealthErr(want)
	if err := rt.Health(ctx); err == nil {
		t.Fatal("expected injected health error")
	}
}

// Interface conformance, including optional capabilities.
var (
	_ workload.ModelRuntime       = (*MockModelRuntime)(nil)
	_ workload.ModelHealthChecker = (*MockModelRuntime)(nil)
	_ workload.ModelStatsReporter = (*MockModelRuntime)(nil)
	_ workload.ModelLister        = (*MockModelRuntime)(nil)
	_ workload.ModelRemover       = (*MockModelRuntime)(nil)
)

func TestMockModelRuntimeIsolatesLabels(t *testing.T) {
	t.Parallel()
	rt := NewMockModelRuntime()
	ctx := context.Background()

	labels := map[string]string{"team": "platform"}
	if _, err := rt.Start(ctx, workload.ModelSpec{Ref: "ai/smollm2", Labels: labels}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Mutating the caller's map must not affect stored state.
	labels["team"] = "mutated"

	models, err := rt.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := models[0].Labels["team"]; got != "platform" {
		t.Fatalf("stored label leaked caller mutation: got %q", got)
	}
	// Mutating the returned map must not affect stored state either.
	models[0].Labels["team"] = "again"
	models, _ = rt.ListModels(ctx)
	if got := models[0].Labels["team"]; got != "platform" {
		t.Fatalf("returned label leaked into stored state: got %q", got)
	}
}

func TestMockModelRuntimeListDeterministicOrder(t *testing.T) {
	t.Parallel()
	rt := NewMockModelRuntime()
	ctx := context.Background()
	for _, ref := range []string{"ai/c", "ai/a", "ai/b"} {
		if _, err := rt.Start(ctx, workload.ModelSpec{Ref: ref}); err != nil {
			t.Fatalf("Start %s: %v", ref, err)
		}
	}
	models, err := rt.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	got := []string{models[0].Ref, models[1].Ref, models[2].Ref}
	want := []string{"ai/a", "ai/b", "ai/c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected sorted refs %v, got %v", want, got)
		}
	}
}
