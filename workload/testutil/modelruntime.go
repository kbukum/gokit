package testutil

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/kbukum/gokit/workload"
)

// MockModelRuntime is an in-memory implementation of [workload.ModelRuntime]
// (and the optional [workload.ModelHealthChecker], [workload.ModelStatsReporter],
// [workload.ModelLister], and [workload.ModelRemover] capabilities) for testing
// consumers without a real model runtime daemon.
//
// Started models are tracked in memory; Endpoint returns a deterministic fake
// base URL. Inject a health error with [MockModelRuntime.SetHealthErr] to
// exercise failure paths.
type MockModelRuntime struct {
	baseURL   string
	mu        sync.RWMutex
	models    map[string]*mockModel
	healthErr error
}

type mockModel struct {
	spec   workload.ModelSpec
	loaded bool
}

var (
	_ workload.ModelRuntime       = (*MockModelRuntime)(nil)
	_ workload.ModelHealthChecker = (*MockModelRuntime)(nil)
	_ workload.ModelStatsReporter = (*MockModelRuntime)(nil)
	_ workload.ModelLister        = (*MockModelRuntime)(nil)
	_ workload.ModelRemover       = (*MockModelRuntime)(nil)
)

// NewMockModelRuntime creates an empty in-memory model runtime.
func NewMockModelRuntime() *MockModelRuntime {
	return &MockModelRuntime{
		baseURL: "http://mock-model-runtime/engines/v1",
		models:  make(map[string]*mockModel),
	}
}

// SetHealthErr makes Health return err (nil clears it).
func (m *MockModelRuntime) SetHealthErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthErr = err
}

// StartCount returns how many distinct models are tracked.
func (m *MockModelRuntime) StartCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.models)
}

// Start records the model as loaded and returns its handle. The spec's mutable
// maps are cloned so later caller mutation cannot race the fake's state.
func (m *MockModelRuntime) Start(_ context.Context, spec workload.ModelSpec) (*workload.ModelHandle, error) {
	if spec.Ref == "" {
		return nil, fmt.Errorf("mock model runtime: model ref is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	spec.Labels = maps.Clone(spec.Labels)
	spec.Metadata = maps.Clone(spec.Metadata)
	m.models[spec.Ref] = &mockModel{spec: spec, loaded: true}
	return &workload.ModelHandle{
		Model:    spec.Ref,
		Status:   workload.StatusRunning,
		Endpoint: m.endpoint(spec.Ref),
	}, nil
}

// Stop marks the model as unloaded. Unknown models are a no-op.
func (m *MockModelRuntime) Stop(_ context.Context, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mm, ok := m.models[model]; ok {
		mm.loaded = false
	}
	return nil
}

// Health returns the injected error, if any.
func (m *MockModelRuntime) Health(_ context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthErr
}

// Endpoint returns the fake endpoint for a model. Mirroring the real backends,
// it is a pure address computation that does not require the model to have been
// started; it errors only on an empty model.
func (m *MockModelRuntime) Endpoint(_ context.Context, model string) (*workload.Endpoint, error) {
	if model == "" {
		return nil, fmt.Errorf("mock model runtime: model is required")
	}
	ep := m.endpoint(model)
	return &ep, nil
}

// Stats reports the loaded state for a known model.
func (m *MockModelRuntime) Stats(_ context.Context, model string) (*workload.ModelStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mm, ok := m.models[model]
	if !ok {
		return nil, fmt.Errorf("mock model runtime: model %q not started", model)
	}
	return &workload.ModelStats{Loaded: mm.loaded}, nil
}

// ListModels returns tracked models in deterministic (ref-sorted) order. Label
// maps are cloned so callers cannot mutate the fake's internal state.
func (m *MockModelRuntime) ListModels(_ context.Context) ([]workload.ModelInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	refs := make([]string, 0, len(m.models))
	for ref := range m.models {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	out := make([]workload.ModelInfo, 0, len(refs))
	for _, ref := range refs {
		out = append(out, workload.ModelInfo{ID: ref, Ref: ref, Labels: maps.Clone(m.models[ref].spec.Labels)})
	}
	return out, nil
}

// RemoveModel deletes a tracked model.
func (m *MockModelRuntime) RemoveModel(_ context.Context, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.models[model]; !ok {
		return fmt.Errorf("mock model runtime: model %q not found", model)
	}
	delete(m.models, model)
	return nil
}

// endpoint builds the deterministic fake endpoint. baseURL is immutable after
// construction, so no lock is required.
func (m *MockModelRuntime) endpoint(model string) workload.Endpoint {
	return workload.Endpoint{BaseURL: m.baseURL, Model: model, API: workload.APIOpenAI}
}
