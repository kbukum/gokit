package workload

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// ---------------------------------------------------------------------------
// Test-local mock Manager
// ---------------------------------------------------------------------------

type mockWorkload struct {
	req    DeployRequest
	status string
}

type mockManager struct {
	workloads map[string]*mockWorkload
	nextID    int
	mu        sync.RWMutex
	healthy   bool
}

func newMockManager() *mockManager {
	return &mockManager{workloads: make(map[string]*mockWorkload), healthy: true}
}

var _ Manager = (*mockManager)(nil)

func (m *mockManager) Deploy(_ context.Context, req DeployRequest) (*DeployResult, error) {
	if req.Image == "" {
		return nil, fmt.Errorf("image is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("wl-%d", m.nextID)
	m.workloads[id] = &mockWorkload{req: req, status: StatusRunning}
	return &DeployResult{ID: id, Name: req.Name, Status: StatusRunning}, nil
}

func (m *mockManager) Stop(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	wl, ok := m.workloads[id]
	if !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	wl.status = StatusStopped
	return nil
}

func (m *mockManager) Remove(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workloads[id]; !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	delete(m.workloads, id)
	return nil
}

func (m *mockManager) Restart(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	wl, ok := m.workloads[id]
	if !ok {
		return fmt.Errorf("workload %q not found", id)
	}
	wl.status = StatusRunning
	return nil
}

func (m *mockManager) Status(_ context.Context, id string) (*WorkloadStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wl, ok := m.workloads[id]
	if !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return &WorkloadStatus{
		ID:      id,
		Name:    wl.req.Name,
		Image:   wl.req.Image,
		Status:  wl.status,
		Running: wl.status == StatusRunning,
	}, nil
}

func (m *mockManager) Wait(_ context.Context, id string) (*WaitResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workloads[id]; !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return &WaitResult{StatusCode: 0}, nil
}

func (m *mockManager) Logs(_ context.Context, id string, _ LogOptions) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workloads[id]; !ok {
		return nil, fmt.Errorf("workload %q not found", id)
	}
	return []string{"line 1", "line 2"}, nil
}

func (m *mockManager) List(_ context.Context, filter ListFilter) ([]WorkloadInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []WorkloadInfo
	for id, wl := range m.workloads {
		if filter.Status != "" && wl.status != filter.Status {
			continue
		}
		if filter.Name != "" && !strings.HasPrefix(wl.req.Name, filter.Name) {
			continue
		}
		result = append(result, WorkloadInfo{
			ID:     id,
			Name:   wl.req.Name,
			Image:  wl.req.Image,
			Status: wl.status,
			Labels: wl.req.Labels,
		})
	}
	return result, nil
}

func (m *mockManager) HealthCheck(_ context.Context) error {
	if !m.healthy {
		return fmt.Errorf("unhealthy")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func testLogger() *logger.Logger {
	cfg := &logger.Config{Level: "error", Format: "json"}
	return logger.New(cfg, "workload-test")
}

// ===========================================================================
// Status constants
// ===========================================================================

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{"Created", StatusCreated},
		{"Running", StatusRunning},
		{"Stopped", StatusStopped},
		{"Completed", StatusCompleted},
		{"Error", StatusError},
		{"Restarting", StatusRestarting},
		{"Unknown", StatusUnknown},
		{"NotFound", StatusNotFound},
	}
	seen := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.val == "" {
				t.Errorf("status %s should not be empty", tt.name)
			}
			if seen[tt.val] {
				t.Errorf("duplicate status value: %s", tt.val)
			}
			seen[tt.val] = true
		})
	}
}

func TestProviderConstants(t *testing.T) {
	if ProviderDocker != "docker" {
		t.Errorf("ProviderDocker = %q, want docker", ProviderDocker)
	}
	if ProviderKubernetes != "kubernetes" {
		t.Errorf("ProviderKubernetes = %q, want kubernetes", ProviderKubernetes)
	}
}

// ===========================================================================
// DeployRequest tests
// ===========================================================================

func TestDeployRequest_AllFields(t *testing.T) {
	req := DeployRequest{
		Name:        "my-app",
		Image:       "nginx:latest",
		Command:     []string{"/bin/sh"},
		Args:        []string{"-c", "echo hello"},
		Environment: map[string]string{"FOO": "bar"},
		Labels:      map[string]string{"app": "web"},
		Annotations: map[string]string{"note": "test"},
		WorkDir:     "/app",
		Resources: &ResourceConfig{
			CPULimit:    "500m",
			MemoryLimit: "256m",
		},
		Network: &NetworkConfig{
			Mode: "bridge",
			DNS:  []string{"8.8.8.8"},
		},
		Volumes: []VolumeMount{
			{Source: "/data", Target: "/mnt", ReadOnly: true, Type: "bind"},
		},
		Ports: []PortMapping{
			{Host: 8080, Container: 80, Protocol: "tcp"},
		},
		RestartPolicy:  "always",
		AutoRemove:     false,
		Replicas:       3,
		Timeout:        30 * time.Second,
		Platform:       "linux/amd64",
		Namespace:      "production",
		ServiceAccount: "app-sa",
		Metadata:       map[string]any{"custom": true},
	}

	if req.Name != "my-app" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.Image != "nginx:latest" {
		t.Errorf("Image = %q", req.Image)
	}
	if len(req.Command) != 1 {
		t.Errorf("Command len = %d", len(req.Command))
	}
	if len(req.Args) != 2 {
		t.Errorf("Args len = %d", len(req.Args))
	}
	if req.Environment["FOO"] != "bar" {
		t.Error("Environment[FOO] not set")
	}
	if req.Resources.CPULimit != "500m" {
		t.Errorf("CPULimit = %q", req.Resources.CPULimit)
	}
	if req.Network.Mode != "bridge" {
		t.Errorf("Network.Mode = %q", req.Network.Mode)
	}
	if len(req.Volumes) != 1 || !req.Volumes[0].ReadOnly {
		t.Error("Volume not configured correctly")
	}
	if len(req.Ports) != 1 || req.Ports[0].Host != 8080 {
		t.Error("Port not configured correctly")
	}
	if req.Replicas != 3 {
		t.Errorf("Replicas = %d", req.Replicas)
	}
	if req.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", req.Timeout)
	}
	if req.Namespace != "production" {
		t.Errorf("Namespace = %q", req.Namespace)
	}
}

func TestDeployRequest_MinimalFields(t *testing.T) {
	req := DeployRequest{
		Name:  "simple",
		Image: "alpine",
	}
	if req.Name != "simple" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.Image != "alpine" {
		t.Errorf("Image = %q", req.Image)
	}
	// Zero-value defaults
	if req.Command != nil {
		t.Error("Command should be nil")
	}
	if req.Environment != nil {
		t.Error("Environment should be nil")
	}
	if req.Resources != nil {
		t.Error("Resources should be nil")
	}
	if req.Network != nil {
		t.Error("Network should be nil")
	}
	if req.Replicas != 0 {
		t.Errorf("Replicas = %d, want 0", req.Replicas)
	}
	if req.AutoRemove {
		t.Error("AutoRemove should be false")
	}
}

func TestDeployRequest_LargeEnvMap(t *testing.T) {
	env := make(map[string]string, 200)
	for i := 0; i < 200; i++ {
		env[fmt.Sprintf("VAR_%d", i)] = fmt.Sprintf("val_%d", i)
	}
	req := DeployRequest{Name: "big-env", Image: "app", Environment: env}
	if len(req.Environment) != 200 {
		t.Errorf("Environment len = %d, want 200", len(req.Environment))
	}
}

func TestDeployRequest_LongArgs(t *testing.T) {
	args := make([]string, 100)
	for i := range args {
		args[i] = strings.Repeat("a", 1000)
	}
	req := DeployRequest{Name: "long-args", Image: "app", Args: args}
	if len(req.Args) != 100 {
		t.Errorf("Args len = %d", len(req.Args))
	}
}

// ===========================================================================
// DeployResult, WaitResult, WorkloadStatus, WorkloadInfo
// ===========================================================================

func TestDeployResult_Fields(t *testing.T) {
	r := DeployResult{ID: "abc123", Name: "web", Status: StatusRunning}
	if r.ID != "abc123" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Status != StatusRunning {
		t.Errorf("Status = %q", r.Status)
	}
}

func TestWaitResult_Success(t *testing.T) {
	r := WaitResult{StatusCode: 0}
	if r.StatusCode != 0 {
		t.Errorf("StatusCode = %d", r.StatusCode)
	}
	if r.Error != "" {
		t.Errorf("Error = %q", r.Error)
	}
}

func TestWaitResult_WithError(t *testing.T) {
	r := WaitResult{StatusCode: 1, Error: "OOM killed"}
	if r.StatusCode != 1 {
		t.Errorf("StatusCode = %d", r.StatusCode)
	}
	if r.Error != "OOM killed" {
		t.Errorf("Error = %q", r.Error)
	}
}

func TestWorkloadStatus_Fields(t *testing.T) {
	now := time.Now()
	s := WorkloadStatus{
		ID:        "id-1",
		Name:      "web",
		Image:     "nginx",
		Status:    StatusRunning,
		Running:   true,
		Healthy:   true,
		Ready:     true,
		StartedAt: now,
		ExitCode:  0,
		Message:   "ok",
		Restarts:  2,
	}
	if s.ID != "id-1" {
		t.Errorf("ID = %q", s.ID)
	}
	if !s.Running {
		t.Error("Running should be true")
	}
	if !s.Healthy {
		t.Error("Healthy should be true")
	}
	if !s.Ready {
		t.Error("Ready should be true")
	}
	if s.StartedAt != now {
		t.Error("StartedAt mismatch")
	}
	if s.Restarts != 2 {
		t.Errorf("Restarts = %d", s.Restarts)
	}
}

func TestWorkloadInfo_Fields(t *testing.T) {
	now := time.Now()
	info := WorkloadInfo{
		ID:        "id-1",
		Name:      "api",
		Image:     "myapp:v1",
		Status:    StatusRunning,
		Labels:    map[string]string{"env": "prod"},
		Created:   now,
		Namespace: "default",
	}
	if info.Labels["env"] != "prod" {
		t.Error("Labels not set")
	}
	if info.Created != now {
		t.Error("Created mismatch")
	}
}

func TestWorkloadStats_Fields(t *testing.T) {
	s := WorkloadStats{
		CPUPercent:     55.5,
		MemoryUsage:    1024 * 1024 * 100,
		MemoryLimit:    1024 * 1024 * 512,
		NetworkRxBytes: 1000,
		NetworkTxBytes: 2000,
		PIDs:           10,
	}
	if s.CPUPercent != 55.5 {
		t.Errorf("CPUPercent = %f", s.CPUPercent)
	}
	if s.PIDs != 10 {
		t.Errorf("PIDs = %d", s.PIDs)
	}
}

func TestWorkloadEvent_Fields(t *testing.T) {
	e := WorkloadEvent{
		ID:        "id-1",
		Name:      "web",
		Event:     "start",
		Timestamp: time.Now(),
		Message:   "started",
	}
	if e.Event != "start" {
		t.Errorf("Event = %q", e.Event)
	}
}

func TestLogOptions_Defaults(t *testing.T) {
	opts := LogOptions{}
	if opts.Tail != 0 {
		t.Errorf("Tail = %d", opts.Tail)
	}
	if opts.Follow {
		t.Error("Follow should be false")
	}
}

func TestListFilter_Defaults(t *testing.T) {
	f := ListFilter{}
	if f.Labels != nil {
		t.Error("Labels should be nil")
	}
	if f.Name != "" {
		t.Errorf("Name = %q", f.Name)
	}
	if f.Status != "" {
		t.Errorf("Status = %q", f.Status)
	}
}

func TestExecResult_Fields(t *testing.T) {
	r := ExecResult{ExitCode: 0, Stdout: "hello", Stderr: ""}
	if r.ExitCode != 0 {
		t.Errorf("ExitCode = %d", r.ExitCode)
	}
	if r.Stdout != "hello" {
		t.Errorf("Stdout = %q", r.Stdout)
	}
}

// ===========================================================================
// ResourceConfig, NetworkConfig, PortMapping, VolumeMount
// ===========================================================================

func TestResourceConfig_Fields(t *testing.T) {
	rc := ResourceConfig{
		CPULimit:      "1",
		CPURequest:    "500m",
		MemoryLimit:   "1g",
		MemoryRequest: "512m",
	}
	if rc.CPULimit != "1" {
		t.Errorf("CPULimit = %q", rc.CPULimit)
	}
	if rc.MemoryRequest != "512m" {
		t.Errorf("MemoryRequest = %q", rc.MemoryRequest)
	}
}

func TestNetworkConfig_Fields(t *testing.T) {
	nc := NetworkConfig{
		Mode:  "host",
		DNS:   []string{"8.8.8.8", "1.1.1.1"},
		Hosts: map[string]string{"myhost": "127.0.0.1"},
	}
	if nc.Mode != "host" {
		t.Errorf("Mode = %q", nc.Mode)
	}
	if len(nc.DNS) != 2 {
		t.Errorf("DNS len = %d", len(nc.DNS))
	}
}

func TestPortMapping_Fields(t *testing.T) {
	pm := PortMapping{Host: 8080, Container: 80, Protocol: "tcp"}
	if pm.Host != 8080 {
		t.Errorf("Host = %d", pm.Host)
	}
	if pm.Container != 80 {
		t.Errorf("Container = %d", pm.Container)
	}
	if pm.Protocol != "tcp" {
		t.Errorf("Protocol = %q", pm.Protocol)
	}
}

func TestPortMapping_UDP(t *testing.T) {
	pm := PortMapping{Host: 53, Container: 53, Protocol: "udp"}
	if pm.Protocol != "udp" {
		t.Errorf("Protocol = %q", pm.Protocol)
	}
}

func TestVolumeMount_Bind(t *testing.T) {
	vm := VolumeMount{Source: "/host/data", Target: "/container/data", ReadOnly: false, Type: "bind"}
	if vm.Source != "/host/data" {
		t.Errorf("Source = %q", vm.Source)
	}
	if vm.ReadOnly {
		t.Error("ReadOnly should be false")
	}
}

func TestVolumeMount_ReadOnly(t *testing.T) {
	vm := VolumeMount{Source: "config", Target: "/etc/config", ReadOnly: true, Type: "configmap"}
	if !vm.ReadOnly {
		t.Error("ReadOnly should be true")
	}
	if vm.Type != "configmap" {
		t.Errorf("Type = %q", vm.Type)
	}
}

// ===========================================================================
// Manager interface tests (via mock)
// ===========================================================================

func TestManager_DeployReturnsID(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	result, err := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if result.ID == "" {
		t.Error("Deploy should return non-empty ID")
	}
	if result.Name != "app" {
		t.Errorf("Name = %q", result.Name)
	}
	if result.Status != StatusRunning {
		t.Errorf("Status = %q", result.Status)
	}
}

func TestManager_DeployEmptyImage(t *testing.T) {
	mgr := newMockManager()
	_, err := mgr.Deploy(context.Background(), DeployRequest{Name: "app", Image: ""})
	if err == nil {
		t.Error("Deploy with empty image should fail")
	}
}

func TestManager_DeployEmptyName(t *testing.T) {
	mgr := newMockManager()
	_, err := mgr.Deploy(context.Background(), DeployRequest{Name: "", Image: "nginx"})
	if err == nil {
		t.Error("Deploy with empty name should fail")
	}
}

func TestManager_StopRunningWorkload(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	if err := mgr.Stop(ctx, res.ID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status, _ := mgr.Status(ctx, res.ID)
	if status.Status != StatusStopped {
		t.Errorf("Status after Stop = %q, want %q", status.Status, StatusStopped)
	}
	if status.Running {
		t.Error("Running should be false after Stop")
	}
}

func TestManager_StopNonExistent(t *testing.T) {
	mgr := newMockManager()
	err := mgr.Stop(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Stop non-existent workload should fail")
	}
}

func TestManager_RemoveWorkload(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	if err := mgr.Remove(ctx, res.ID); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err := mgr.Status(ctx, res.ID)
	if err == nil {
		t.Error("Status after Remove should fail")
	}
}

func TestManager_RemoveNonExistent(t *testing.T) {
	mgr := newMockManager()
	err := mgr.Remove(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Remove non-existent workload should fail")
	}
}

func TestManager_RemoveAlreadyRemoved(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	mgr.Remove(ctx, res.ID)
	err := mgr.Remove(ctx, res.ID)
	if err == nil {
		t.Error("Removing already-removed workload should fail")
	}
}

func TestManager_RestartWorkload(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	mgr.Stop(ctx, res.ID)

	if err := mgr.Restart(ctx, res.ID); err != nil {
		t.Fatalf("Restart failed: %v", err)
	}

	status, _ := mgr.Status(ctx, res.ID)
	if status.Status != StatusRunning {
		t.Errorf("Status after Restart = %q, want %q", status.Status, StatusRunning)
	}
}

func TestManager_RestartNonExistent(t *testing.T) {
	mgr := newMockManager()
	err := mgr.Restart(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Restart non-existent workload should fail")
	}
}

func TestManager_StatusRunning(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "web", Image: "nginx"})
	status, err := mgr.Status(ctx, res.ID)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.Status != StatusRunning {
		t.Errorf("Status = %q", status.Status)
	}
	if !status.Running {
		t.Error("Running should be true")
	}
	if status.Name != "web" {
		t.Errorf("Name = %q", status.Name)
	}
	if status.Image != "nginx" {
		t.Errorf("Image = %q", status.Image)
	}
}

func TestManager_StatusNonExistent(t *testing.T) {
	mgr := newMockManager()
	_, err := mgr.Status(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Status for non-existent workload should fail")
	}
}

func TestManager_Wait(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "job", Image: "busybox"})
	wr, err := mgr.Wait(ctx, res.ID)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if wr.StatusCode != 0 {
		t.Errorf("StatusCode = %d", wr.StatusCode)
	}
}

func TestManager_WaitNonExistent(t *testing.T) {
	mgr := newMockManager()
	_, err := mgr.Wait(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Wait for non-existent workload should fail")
	}
}

func TestManager_Logs(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res, _ := mgr.Deploy(ctx, DeployRequest{Name: "app", Image: "nginx"})
	logs, err := mgr.Logs(ctx, res.ID, LogOptions{Tail: 10})
	if err != nil {
		t.Fatalf("Logs failed: %v", err)
	}
	if len(logs) == 0 {
		t.Error("Logs should return at least one line")
	}
}

func TestManager_LogsNonExistent(t *testing.T) {
	mgr := newMockManager()
	_, err := mgr.Logs(context.Background(), "nonexistent", LogOptions{})
	if err == nil {
		t.Error("Logs for non-existent workload should fail")
	}
}

func TestManager_List(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	mgr.Deploy(ctx, DeployRequest{Name: "web-1", Image: "nginx"})
	mgr.Deploy(ctx, DeployRequest{Name: "web-2", Image: "nginx"})

	items, err := mgr.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("List count = %d, want 2", len(items))
	}
}

func TestManager_ListWithStatusFilter(t *testing.T) {
	mgr := newMockManager()
	ctx := context.Background()

	res1, _ := mgr.Deploy(ctx, DeployRequest{Name: "web-1", Image: "nginx"})
	mgr.Deploy(ctx, DeployRequest{Name: "web-2", Image: "nginx"})
	mgr.Stop(ctx, res1.ID)

	running, _ := mgr.List(ctx, ListFilter{Status: StatusRunning})
	if len(running) != 1 {
		t.Errorf("running count = %d, want 1", len(running))
	}

	stopped, _ := mgr.List(ctx, ListFilter{Status: StatusStopped})
	if len(stopped) != 1 {
		t.Errorf("stopped count = %d, want 1", len(stopped))
	}
}

func TestManager_HealthCheck(t *testing.T) {
	mgr := newMockManager()
	if err := mgr.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestManager_HealthCheckUnhealthy(t *testing.T) {
	mgr := newMockManager()
	mgr.healthy = false
	if err := mgr.HealthCheck(context.Background()); err == nil {
		t.Error("HealthCheck should fail when unhealthy")
	}
}

// ===========================================================================
// Config tests
// ===========================================================================

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	if cfg.Provider != DefaultProvider {
		t.Errorf("Provider = %q, want %q", cfg.Provider, DefaultProvider)
	}
}

func TestConfig_ApplyDefaultsPreservesExisting(t *testing.T) {
	cfg := Config{Provider: "kubernetes"}
	cfg.ApplyDefaults()
	if cfg.Provider != "kubernetes" {
		t.Errorf("Provider = %q, want kubernetes", cfg.Provider)
	}
}

func TestConfig_ValidateOK(t *testing.T) {
	cfg := Config{Provider: "docker"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestConfig_ValidateEmptyProvider(t *testing.T) {
	cfg := Config{Provider: ""}
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate should fail with empty provider")
	}
	if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_DefaultLabels(t *testing.T) {
	cfg := Config{
		Provider:      "docker",
		DefaultLabels: map[string]string{"team": "platform"},
	}
	if cfg.DefaultLabels["team"] != "platform" {
		t.Error("DefaultLabels not set")
	}
}

// ===========================================================================
// Factory tests
// ===========================================================================

func TestFactory_RegisterAndCreate(t *testing.T) {
	reg := NewFactoryRegistry()
	mustReg(t, reg, "test-provider", func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
		return newMockManager(), nil
	})

	cfg := Config{Provider: "test-provider"}
	mgr, err := New(reg, cfg, nil, testLogger())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if mgr == nil {
		t.Error("New returned nil manager")
	}
}

func TestFactory_UnknownProvider(t *testing.T) {
	reg := NewFactoryRegistry()

	cfg := Config{Provider: "nonexistent"}
	_, err := New(reg, cfg, nil, testLogger())
	if err == nil {
		t.Error("New should fail for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFactory_InvalidConfig(t *testing.T) {
	reg := NewFactoryRegistry()

	// Empty provider, even after ApplyDefaults docker won't be registered
	cfg := Config{Provider: ""}
	_, err := New(reg, cfg, nil, testLogger())
	if err == nil {
		t.Error("New should fail for invalid config")
	}
}

// ===========================================================================
// Component tests
// ===========================================================================

func TestComponent_Name(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker"}, nil, testLogger())
	if comp.Name() != "workload" {
		t.Errorf("Name = %q", comp.Name())
	}
}

func TestComponent_ManagerNilBeforeStart(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker"}, nil, testLogger())
	if comp.Manager() != nil {
		t.Error("Manager should be nil before Start")
	}
}

func TestComponent_StartDisabled(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker", Enabled: false}, nil, testLogger())
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if comp.Manager() != nil {
		t.Error("Manager should be nil when disabled")
	}
}

func TestComponent_StartWithRegisteredFactory(t *testing.T) {
	reg := NewFactoryRegistry()
	mustReg(t, reg, "mock", func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
		return newMockManager(), nil
	})

	comp := NewComponent(reg, Config{Provider: "mock", Enabled: true}, nil, testLogger())
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if comp.Manager() == nil {
		t.Error("Manager should not be nil after Start")
	}
}

func TestComponent_StartUnknownProvider(t *testing.T) {
	reg := NewFactoryRegistry()

	comp := NewComponent(reg, Config{Provider: "unknown", Enabled: true}, nil, testLogger())
	err := comp.Start(context.Background())
	if err == nil {
		t.Error("Start with unknown provider should fail")
	}
}

func TestComponent_Stop(t *testing.T) {
	reg := NewFactoryRegistry()
	mustReg(t, reg, "mock", func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
		return newMockManager(), nil
	})

	comp := NewComponent(reg, Config{Provider: "mock", Enabled: true}, nil, testLogger())
	comp.Start(context.Background())
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if comp.Manager() != nil {
		t.Error("Manager should be nil after Stop")
	}
}

func TestComponent_HealthDisabled(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker", Enabled: false}, nil, testLogger())
	h := comp.Health(context.Background())
	if h.Status != component.StatusHealthy {
		t.Errorf("Health.Status = %q, want %q", h.Status, component.StatusHealthy)
	}
	if h.Message != "disabled" {
		t.Errorf("Health.Message = %q", h.Message)
	}
}

func TestComponent_HealthNotInitialized(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker", Enabled: true}, nil, testLogger())
	h := comp.Health(context.Background())
	if h.Status != component.StatusUnhealthy {
		t.Errorf("Health.Status = %q, want %q", h.Status, component.StatusUnhealthy)
	}
	if !strings.Contains(h.Message, "not initialized") {
		t.Errorf("Health.Message = %q", h.Message)
	}
}

func TestComponent_HealthWithManager(t *testing.T) {
	reg := NewFactoryRegistry()
	mustReg(t, reg, "mock", func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
		return newMockManager(), nil
	})

	comp := NewComponent(reg, Config{Provider: "mock", Enabled: true}, nil, testLogger())
	comp.Start(context.Background())
	defer comp.Stop(context.Background())

	h := comp.Health(context.Background())
	if h.Status != component.StatusHealthy {
		t.Errorf("Health.Status = %q, want %q", h.Status, component.StatusHealthy)
	}
}

func TestComponent_HealthCheckFails(t *testing.T) {
	reg := NewFactoryRegistry()
	unhealthy := newMockManager()
	unhealthy.healthy = false
	mustReg(t, reg, "mock", func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
		return unhealthy, nil
	})

	comp := NewComponent(reg, Config{Provider: "mock", Enabled: true}, nil, testLogger())
	comp.Start(context.Background())
	defer comp.Stop(context.Background())

	h := comp.Health(context.Background())
	if h.Status != component.StatusUnhealthy {
		t.Errorf("Health.Status = %q, want %q", h.Status, component.StatusUnhealthy)
	}
	if !strings.Contains(h.Message, "health check failed") {
		t.Errorf("Health.Message = %q", h.Message)
	}
}

func TestComponent_Describe(t *testing.T) {
	reg := NewFactoryRegistry()
	comp := NewComponent(reg, Config{Provider: "docker"}, nil, testLogger())
	desc := comp.Describe()
	if desc.Name != "Workload" {
		t.Errorf("Describe.Name = %q", desc.Name)
	}
	if desc.Type != "workload" {
		t.Errorf("Describe.Type = %q", desc.Type)
	}
	if !strings.Contains(desc.Details, "docker") {
		t.Errorf("Describe.Details = %q", desc.Details)
	}
}

// ===========================================================================
// Resource parsing tests
// ===========================================================================

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1024", 1024},
		{"1k", 1024},
		{"1ki", 1024},
		{"512m", 512 * 1024 * 1024},
		{"512mi", 512 * 1024 * 1024},
		{"2g", 2 * 1024 * 1024 * 1024},
		{"2gi", 2 * 1024 * 1024 * 1024},
		{"1t", 1024 * 1024 * 1024 * 1024},
		{"1ti", 1024 * 1024 * 1024 * 1024},
		{"  512m  ", 512 * 1024 * 1024},
		{"0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemory(tt.input)
			if err != nil {
				t.Fatalf("ParseMemory(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMemory(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMemory_Errors(t *testing.T) {
	tests := []struct {
		input   string
		wantMsg string
	}{
		{"", "empty memory string"},
		{"abc", "parse memory"},
		{"-1m", "non-negative"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseMemory(tt.input)
			if err == nil {
				t.Fatalf("ParseMemory(%q) should fail", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ParseMemory(%q) error = %v, want containing %q", tt.input, err, tt.wantMsg)
			}
		})
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"500m", 500_000_000},
		{"1", 1_000_000_000},
		{"0.5", 500_000_000},
		{"2", 2_000_000_000},
		{"1000m", 1_000_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseCPU(tt.input)
			if err != nil {
				t.Fatalf("ParseCPU(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseCPU(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCPU_Errors(t *testing.T) {
	tests := []struct {
		input   string
		wantMsg string
	}{
		{"", "empty CPU string"},
		{"abc", "parse CPU"},
		{"xyzm", "parse CPU"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseCPU(tt.input)
			if err == nil {
				t.Fatalf("ParseCPU(%q) should fail", tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ParseCPU(%q) error = %v, want containing %q", tt.input, err, tt.wantMsg)
			}
		})
	}
}

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{2 * 1024 * 1024 * 1024, "2g"},
		{512 * 1024 * 1024, "512m"},
		{64 * 1024, "64k"},
		{500, "500"},
		{0, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatMemory(tt.input)
			if got != tt.want {
				t.Errorf("FormatMemory(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{1_000_000_000, "1"},
		{2_000_000_000, "2"},
		{500_000_000, "500m"},
		{250_000_000, "250m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatCPU(tt.input)
			if got != tt.want {
				t.Errorf("FormatCPU(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatCPU_Fractional(t *testing.T) {
	got := FormatCPU(1_500_000)
	if !strings.Contains(got, ".") {
		t.Errorf("FormatCPU(1500000) = %q, want fractional format", got)
	}
}

// ===========================================================================
// Interface compliance
// ===========================================================================

func TestComponent_ImplementsComponent(t *testing.T) {
	var _ component.Component = (*Component)(nil)
}

func TestComponent_ImplementsDescribable(t *testing.T) {
	var _ component.Describable = (*Component)(nil)
}

// mustReg registers f on reg, failing the test on error.
func mustReg(tb testing.TB, reg *FactoryRegistry, name string, f ManagerFactory) {
tb.Helper()
if err := reg.Register(name, f); err != nil {
tb.Fatalf("register: %v", err)
}
}
