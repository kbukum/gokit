package discovery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/logger"
)

// ── helpers ─────────────────────────────────────────────────────────

func makeInstance(id, name, addr string, port int) ServiceInstance {
	return ServiceInstance{
		ID:       id,
		Name:     name,
		Address:  addr,
		Port:     port,
		Protocol: "grpc",
		Tags:     []string{"prod"},
		Metadata: map[string]string{"version": "1"},
		Health:   HealthHealthy,
		Weight:   1,
		LastSeen: time.Now(),
	}
}

// mockDiscovery implements Discovery for testing the Client.
type mockDiscovery struct {
	mu        sync.Mutex
	instances map[string][]ServiceInstance
	err       error
	calls     int
}

func (m *mockDiscovery) Discover(_ context.Context, serviceName string) ([]ServiceInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.instances[serviceName], nil
}

func (m *mockDiscovery) Watch(ctx context.Context, _ string) (<-chan []ServiceInstance, error) {
	ch := make(chan []ServiceInstance)
	go func() { <-ctx.Done(); close(ch) }()
	return ch, nil
}

func (m *mockDiscovery) Close() error { return nil }

// testLogger creates a minimal logger for testing.
func testLogger() *logger.Logger {
	return logger.New(&logger.Config{Level: "error", Format: "json"}, "test")
}

// ── ServiceInstance tests ───────────────────────────────────────────

func TestServiceInstance_Creation(t *testing.T) {
	inst := makeInstance("svc-1", "api", "10.0.0.1", 8080)
	if inst.ID != "svc-1" {
		t.Errorf("ID = %q, want %q", inst.ID, "svc-1")
	}
	if inst.Name != "api" {
		t.Errorf("Name = %q, want %q", inst.Name, "api")
	}
	if inst.Address != "10.0.0.1" {
		t.Errorf("Address = %q, want %q", inst.Address, "10.0.0.1")
	}
	if inst.Port != 8080 {
		t.Errorf("Port = %d, want %d", inst.Port, 8080)
	}
}

func TestServiceInstance_HostPort(t *testing.T) {
	inst := makeInstance("id", "svc", "192.168.1.5", 9090)
	got := inst.HostPort()
	if got != "192.168.1.5:9090" {
		t.Errorf("HostPort() = %q, want %q", got, "192.168.1.5:9090")
	}
}

func TestServiceInstance_MetadataPreservation(t *testing.T) {
	inst := ServiceInstance{
		ID:       "m1",
		Metadata: map[string]string{"region": "us-east-1", "env": "staging"},
	}
	if inst.Metadata["region"] != "us-east-1" {
		t.Errorf("metadata[region] = %q, want %q", inst.Metadata["region"], "us-east-1")
	}
	if inst.Metadata["env"] != "staging" {
		t.Errorf("metadata[env] = %q, want %q", inst.Metadata["env"], "staging")
	}
}

func TestHealthStatus_Values(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
		{HealthUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("HealthStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

// ── ServiceResolver extended tests ──────────────────────────────────

func TestServiceResolver_AllowlistBlocks(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"allowed": {Address: "10.0.0.1", Port: 80},
			"blocked": {Address: "10.0.0.2", Port: 80},
		},
	}
	resolver := NewServiceResolver(disc, []string{"allowed"})

	if _, err := resolver.Resolve("blocked"); err == nil {
		t.Fatal("expected error for non-allowlisted service")
	}
}

func TestServiceResolver_EmptyAllowlistPermitsAll(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"any": {Address: "10.0.0.1", Port: 80},
		},
	}
	resolver := NewServiceResolver(disc, nil)

	url, err := resolver.Resolve("any")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://10.0.0.1:80" {
		t.Errorf("got %q, want %q", url, "http://10.0.0.1:80")
	}
}

func TestServiceResolver_WithStrategyOption(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"svc": {Address: "10.0.0.1", Port: 80},
		},
	}
	resolver := NewServiceResolver(disc, nil, WithStrategy(RoundRobin))

	url, err := resolver.Resolve("svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://10.0.0.1:80" {
		t.Errorf("got %q, want %q", url, "http://10.0.0.1:80")
	}
}

// ── Client and cache behavior ───────────────────────────────────────

func TestClient_CacheBehavior(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {makeInstance("a", "svc", "10.0.0.1", 80)},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: 5 * time.Second}, testLogger())
	defer client.Close()

	ctx := context.Background()

	// First call fetches from backend
	_, err := client.Discover(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}

	// Second call should use cache (no additional backend calls)
	_, err = client.Discover(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}

	mock.mu.Lock()
	calls := mock.calls
	mock.mu.Unlock()

	if calls != 1 {
		t.Errorf("backend calls = %d, want 1 (cache should be used)", calls)
	}
}

func TestClient_Invalidate(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {makeInstance("a", "svc", "10.0.0.1", 80)},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: 5 * time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()

	client.Discover(ctx, "svc")
	client.Invalidate("svc")
	client.Discover(ctx, "svc")

	mock.mu.Lock()
	calls := mock.calls
	mock.mu.Unlock()

	if calls != 2 {
		t.Errorf("backend calls = %d, want 2 (invalidated cache should cause re-fetch)", calls)
	}
}

func TestClient_DiscoverOne_RoundRobin(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				makeInstance("a", "svc", "10.0.0.1", 80),
				makeInstance("b", "svc", "10.0.0.2", 80),
				makeInstance("c", "svc", "10.0.0.3", 80),
			},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyRoundRobin}

	ids := make([]string, 6)
	for i := 0; i < 6; i++ {
		inst, err := client.DiscoverOne(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = inst.ID
	}

	// Round-robin should cycle: a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, want := range expected {
		if ids[i] != want {
			t.Errorf("pick[%d] = %q, want %q", i, ids[i], want)
		}
	}
}

func TestClient_DiscoverOne_Random(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				makeInstance("a", "svc", "10.0.0.1", 80),
				makeInstance("b", "svc", "10.0.0.2", 80),
				makeInstance("c", "svc", "10.0.0.3", 80),
			},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyRandom}

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		inst, err := client.DiscoverOne(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		seen[inst.ID] = true
	}

	// With 100 picks from 3 instances, we should see at least 2
	if len(seen) < 2 {
		t.Errorf("random only returned %d unique IDs from 100 picks", len(seen))
	}
}

func TestClient_DiscoverOne_Weighted(t *testing.T) {
	heavyInst := makeInstance("heavy", "svc", "10.0.0.1", 80)
	heavyInst.Weight = 100

	lightInst := makeInstance("light", "svc", "10.0.0.2", 80)
	lightInst.Weight = 1

	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {heavyInst, lightInst},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyWeighted}

	heavyCount := 0
	total := 500
	for i := 0; i < total; i++ {
		inst, err := client.DiscoverOne(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if inst.ID == "heavy" {
			heavyCount++
		}
	}

	// The heavy instance (weight 100) should get roughly 100/101 of traffic
	ratio := float64(heavyCount) / float64(total)
	if ratio < 0.80 {
		t.Errorf("heavy instance ratio = %.2f, want >= 0.80", ratio)
	}
}

func TestClient_DiscoverOne_NoInstances(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyRandom}

	_, err := client.DiscoverOne(ctx, q)
	if !errors.Is(err, ErrNoHealthyEndpoints) {
		t.Errorf("expected ErrNoHealthyEndpoints, got %v", err)
	}
}

func TestClient_DiscoverAll(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"api": {makeInstance("a1", "api", "10.0.0.1", 80)},
			"web": {makeInstance("w1", "web", "10.0.0.2", 80)},
		},
	}
	cfg := ClientConfig{
		CacheTTL: time.Minute,
		Services: []string{"api", "web"},
	}
	client := NewClient(mock, cfg, testLogger())
	defer client.Close()

	ctx := context.Background()
	all, err := client.DiscoverAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("DiscoverAll returned %d services, want 2", len(all))
	}
	if len(all["api"]) != 1 || len(all["web"]) != 1 {
		t.Error("expected 1 instance per service")
	}
}

func TestClient_StaticFallback(t *testing.T) {
	mock := &mockDiscovery{
		err: errors.New("backend down"),
	}
	cfg := ClientConfig{
		CacheTTL: time.Minute,
		StaticEndpoints: []StaticEndpoint{
			{Name: "svc", Address: "fallback.local", Port: 9090, Healthy: true},
		},
	}
	client := NewClient(mock, cfg, testLogger())
	defer client.Close()

	ctx := context.Background()
	instances, err := client.Discover(ctx, "svc")
	if err != nil {
		t.Fatalf("expected fallback to work, got error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 fallback instance, got %d", len(instances))
	}
	if instances[0].Address != "fallback.local" {
		t.Errorf("fallback address = %q, want %q", instances[0].Address, "fallback.local")
	}
}

func TestClient_FilterByProtocol(t *testing.T) {
	grpcInst := makeInstance("grpc-1", "svc", "10.0.0.1", 80)
	grpcInst.Protocol = "grpc"

	httpInst := makeInstance("http-1", "svc", "10.0.0.2", 80)
	httpInst.Protocol = "http"

	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {grpcInst, httpInst},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	instances, err := client.Discover(ctx, "svc", "grpc")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].ID != "grpc-1" {
		t.Errorf("expected only grpc instance, got %v", instances)
	}
}

// ── Config validation ───────────────────────────────────────────────

func TestConfig_Validate_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled config should pass validation, got: %v", err)
	}
}

func TestConfig_Validate_MissingServiceName(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Registration: RegistrationConfig{ServicePort: 8080},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing service_name")
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Registration: RegistrationConfig{
			ServiceName: "test",
			ServicePort: 0,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for zero port")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Registration: RegistrationConfig{
			ServiceName: "test",
			ServicePort: 8080,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()
	if cfg.Provider != "static" {
		t.Errorf("default provider = %q, want %q", cfg.Provider, "static")
	}
	if cfg.Health.Type != HealthCheckHTTP {
		t.Errorf("default health type = %q, want %q", cfg.Health.Type, HealthCheckHTTP)
	}
	if cfg.Health.Path != "/healthz" {
		t.Errorf("default health path = %q, want %q", cfg.Health.Path, "/healthz")
	}
	if cfg.Health.Interval != "10s" {
		t.Errorf("default interval = %q, want %q", cfg.Health.Interval, "10s")
	}
}

func TestConfig_ApplyDefaults_RegistrationServiceID(t *testing.T) {
	cfg := Config{
		Registration: RegistrationConfig{ServiceName: "my-svc"},
	}
	cfg.ApplyDefaults()
	if cfg.Registration.ServiceID != "my-svc" {
		t.Errorf("ServiceID = %q, want %q (should default to ServiceName)", cfg.Registration.ServiceID, "my-svc")
	}
}

func TestConfig_BuildClientConfig(t *testing.T) {
	cfg := Config{
		CacheTTL: "15s",
		Services: []DiscoveredService{
			{Name: "api", Criticality: CriticalityRequired},
			{Name: "web", Criticality: CriticalityOptional},
		},
	}
	cc := cfg.BuildClientConfig()
	if cc.CacheTTL != 15*time.Second {
		t.Errorf("CacheTTL = %v, want %v", cc.CacheTTL, 15*time.Second)
	}
	if len(cc.Services) != 2 {
		t.Errorf("Services count = %d, want 2", len(cc.Services))
	}
	if cc.Criticality["api"] != CriticalityRequired {
		t.Errorf("api criticality = %q, want %q", cc.Criticality["api"], CriticalityRequired)
	}
}

func TestParseDuration_Empty(t *testing.T) {
	d := ParseDuration("")
	if d != 0 {
		t.Errorf("ParseDuration('') = %v, want 0", d)
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	d := ParseDuration("not-a-duration")
	if d != 0 {
		t.Errorf("ParseDuration('not-a-duration') = %v, want 0", d)
	}
}

// ── Concurrent resolve calls ────────────────────────────────────────

func TestClient_ConcurrentResolve(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				makeInstance("a", "svc", "10.0.0.1", 80),
				makeInstance("b", "svc", "10.0.0.2", 80),
			},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Discover(ctx, "svc")
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Discover failed: %v", err)
	}
}

func TestClient_ConcurrentDiscoverOne(t *testing.T) {
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				makeInstance("a", "svc", "10.0.0.1", 80),
				makeInstance("b", "svc", "10.0.0.2", 80),
			},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.DiscoverOne(ctx, Query{ServiceName: "svc", Strategy: StrategyRoundRobin})
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent DiscoverOne failed: %v", err)
	}
}

// ── Performance test (many instances) ───────────────────────────────

func TestClient_ManyInstances(t *testing.T) {
	instances := make([]ServiceInstance, 1000)
	for i := range instances {
		instances[i] = makeInstance(
			fmt.Sprintf("inst-%d", i),
			"svc",
			fmt.Sprintf("10.0.%d.%d", i/256, i%256),
			8080,
		)
	}
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{"svc": instances},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 100; i++ {
		_, err := client.DiscoverOne(ctx, Query{ServiceName: "svc", Strategy: StrategyRoundRobin})
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("100 DiscoverOne calls with 1000 instances took %v, expected < 2s", elapsed)
	}
}

// ── Weighted selection: zero weight treated as 1 ────────────────────

func TestClient_WeightedZeroWeight(t *testing.T) {
	inst := makeInstance("zero", "svc", "10.0.0.1", 80)
	inst.Weight = 0

	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {inst},
		},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	got, err := client.DiscoverOne(ctx, Query{ServiceName: "svc", Strategy: StrategyWeighted})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "zero" {
		t.Errorf("expected zero-weight instance to be selectable (treated as weight 1)")
	}
}

// ── RoundRobin fairness ─────────────────────────────────────────────

func TestClient_RoundRobinFairness(t *testing.T) {
	instances := make([]ServiceInstance, 5)
	for i := range instances {
		instances[i] = makeInstance(fmt.Sprintf("n%d", i), "svc", "10.0.0.1", 8080+i)
	}
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{"svc": instances},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyRoundRobin}
	counts := map[string]int{}
	total := 100

	for i := 0; i < total; i++ {
		inst, err := client.DiscoverOne(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		counts[inst.ID]++
	}

	// Each of the 5 instances should get exactly 20 picks
	for _, inst := range instances {
		if counts[inst.ID] != total/len(instances) {
			t.Errorf("instance %q got %d picks, want %d", inst.ID, counts[inst.ID], total/len(instances))
		}
	}
}

// ── Random distribution ──────────────────────────────────────────────

func TestClient_RandomDistribution(t *testing.T) {
	instances := make([]ServiceInstance, 3)
	for i := range instances {
		instances[i] = makeInstance(fmt.Sprintf("r%d", i), "svc", "10.0.0.1", 8080+i)
	}
	mock := &mockDiscovery{
		instances: map[string][]ServiceInstance{"svc": instances},
	}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute}, testLogger())
	defer client.Close()

	ctx := context.Background()
	q := Query{ServiceName: "svc", Strategy: StrategyRandom}
	counts := map[string]int{}
	total := 600

	for i := 0; i < total; i++ {
		inst, err := client.DiscoverOne(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		counts[inst.ID]++
	}

	// Each instance should be roughly 33% of total. Allow 15% deviation.
	expected := float64(total) / float64(len(instances))
	for id, count := range counts {
		deviation := math.Abs(float64(count)-expected) / expected
		if deviation > 0.30 {
			t.Errorf("instance %q: count=%d, expected~%.0f (deviation %.0f%%)", id, count, expected, deviation*100)
		}
	}
}

// ── Discovery errors ────────────────────────────────────────────────

func TestErrSentinels(t *testing.T) {
	if !errors.Is(ErrServiceNotFound, ErrServiceNotFound) {
		t.Error("ErrServiceNotFound sentinel broken")
	}
	if !errors.Is(ErrNoHealthyEndpoints, ErrNoHealthyEndpoints) {
		t.Error("ErrNoHealthyEndpoints sentinel broken")
	}
	if !errors.Is(ErrDiscoveryDisabled, ErrDiscoveryDisabled) {
		t.Error("ErrDiscoveryDisabled sentinel broken")
	}
}

// ── Query and strategy types ─────────────────────────────────────────

func TestLoadBalancingStrategy_Constants(t *testing.T) {
	if Random != StrategyRandom {
		t.Error("Random alias mismatch")
	}
	if RoundRobin != StrategyRoundRobin {
		t.Error("RoundRobin alias mismatch")
	}
	if Weighted != StrategyWeighted {
		t.Error("Weighted alias mismatch")
	}
	if LeastConn != StrategyLeastConn {
		t.Error("LeastConn alias mismatch")
	}
}

func TestCriticality_Constants(t *testing.T) {
	if CriticalityRequired != "required" {
		t.Error("CriticalityRequired mismatch")
	}
	if CriticalityOptional != "optional" {
		t.Error("CriticalityOptional mismatch")
	}
}

// ── Required service discovery failure ──────────────────────────────

func TestClient_RequiredServiceFailure(t *testing.T) {
	mock := &mockDiscovery{err: errors.New("backend down")}
	cfg := ClientConfig{
		CacheTTL:    time.Minute,
		Services:    []string{"critical-svc"},
		Criticality: map[string]Criticality{"critical-svc": CriticalityRequired},
	}
	client := NewClient(mock, cfg, testLogger())
	defer client.Close()

	ctx := context.Background()
	_, err := client.Discover(ctx, "critical-svc")
	if err == nil {
		t.Fatal("expected error for required service discovery failure")
	}
}

func TestClient_OptionalServiceFailure(t *testing.T) {
	mock := &mockDiscovery{err: errors.New("backend down")}
	cfg := ClientConfig{
		CacheTTL:    time.Minute,
		Services:    []string{"optional-svc"},
		Criticality: map[string]Criticality{"optional-svc": CriticalityOptional},
	}
	client := NewClient(mock, cfg, testLogger())
	defer client.Close()

	ctx := context.Background()
	_, err := client.Discover(ctx, "optional-svc")
	if err == nil {
		t.Fatal("expected error for optional service with no fallback")
	}
}

// ── Endpoint alias ──────────────────────────────────────────────────

func TestEndpoint_IsAlias(t *testing.T) {
	e := ServiceInstance{ID: "ep-1"}
	if e.ID != "ep-1" {
		t.Error("Endpoint alias should be interchangeable with ServiceInstance")
	}
}
