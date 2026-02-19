package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
	"github.com/kbukum/gokit/workload"
)

// Component is a test workload component with an in-memory mock manager.
type Component struct {
	mgr     *MockManager
	started bool
	mu      sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new mock workload test component.
func NewComponent() *Component {
	return &Component{
		mgr: &MockManager{workloads: make(map[string]*mockWorkload)},
	}
}

// Manager returns the mock manager.
func (c *Component) Manager() workload.Manager { return c.mgr }

// MockManagerClient returns the underlying MockManager for assertions.
func (c *Component) MockManagerClient() *MockManager { return c.mgr }

// --- component.Component ---

func (c *Component) Name() string { return "workload-test" }

func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return fmt.Errorf("component already started")
	}
	c.started = true
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = false
	return nil
}

func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.started {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// --- testutil.TestComponent ---

func (c *Component) Reset(_ context.Context) error {
	c.mgr.mu.Lock()
	defer c.mgr.mu.Unlock()
	c.mgr.workloads = make(map[string]*mockWorkload)
	c.mgr.nextID = 0
	return nil
}

func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mgr.mu.RLock()
	defer c.mgr.mu.RUnlock()
	snap := make(map[string]*mockWorkload, len(c.mgr.workloads))
	for k, v := range c.mgr.workloads {
		cp := *v
		snap[k] = &cp
	}
	return snap, nil
}

func (c *Component) Restore(_ context.Context, s interface{}) error {
	snap, ok := s.(map[string]*mockWorkload)
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected map[string]*mockWorkload, got %T", s)
	}
	c.mgr.mu.Lock()
	defer c.mgr.mu.Unlock()
	c.mgr.workloads = make(map[string]*mockWorkload, len(snap))
	for k, v := range snap {
		cp := *v
		c.mgr.workloads[k] = &cp
	}
	return nil
}

// --- MockManager ---

type mockWorkload struct {
	req    workload.DeployRequest
	status string
}

// MockManager is an in-memory implementation of workload.Manager.
type MockManager struct {
	workloads map[string]*mockWorkload
	nextID    int
	mu        sync.RWMutex
}

var _ workload.Manager = (*MockManager)(nil)

// DeployCount returns how many workloads have been deployed.
func (m *MockManager) DeployCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workloads)
}

func (m *MockManager) Deploy(_ context.Context, req workload.DeployRequest) (*workload.DeployResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.workloads[id] = &mockWorkload{req: req, status: workload.StatusRunning}
	return &workload.DeployResult{ID: id, Name: req.Name}, nil
}

func (m *MockManager) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	wl, ok := m.workloads[id]
	if !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	wl.status = workload.StatusStopped
	return nil
}

func (m *MockManager) Remove(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workloads[id]; !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	delete(m.workloads, id)
	return nil
}

func (m *MockManager) Restart(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	wl, ok := m.workloads[id]
	if !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	wl.status = workload.StatusRunning
	return nil
}

func (m *MockManager) Status(_ context.Context, id string) (*workload.WorkloadStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wl, ok := m.workloads[id]
	if !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return &workload.WorkloadStatus{
		ID:      id,
		Name:    wl.req.Name,
		Status:  wl.status,
		Running: wl.status == workload.StatusRunning,
	}, nil
}

func (m *MockManager) Wait(_ context.Context, id string) (*workload.WaitResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workloads[id]; !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return &workload.WaitResult{StatusCode: 0}, nil
}

func (m *MockManager) Logs(_ context.Context, id string, _ workload.LogOptions) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workloads[id]; !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return []string{"mock log line"}, nil
}

func (m *MockManager) List(_ context.Context, _ workload.ListFilter) ([]workload.WorkloadInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]workload.WorkloadInfo, 0, len(m.workloads))
	for id, wl := range m.workloads {
		result = append(result, workload.WorkloadInfo{
			ID:      id,
			Name:    wl.req.Name,
			Image:   wl.req.Image,
			Status:  wl.status,
			Created: time.Now(),
		})
	}
	return result, nil
}

func (m *MockManager) HealthCheck(_ context.Context) error { return nil }
