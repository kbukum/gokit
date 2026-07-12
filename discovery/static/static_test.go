package static

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
)

func TestNewProviderFromStaticEndpoints(t *testing.T) {
	t.Parallel()

	endpoints := []discovery.StaticEndpoint{
		{Name: "api", Address: "10.0.0.1", Port: 8080, Protocol: "grpc", Healthy: true},
		{Name: "api", Address: "10.0.0.2", Port: 8081, Protocol: "http", Tags: []string{"custom"}, Metadata: map[string]string{"protocol": "custom"}, Weight: 5, Healthy: false},
		{Name: "empty", Port: 9090, Healthy: false},
	}
	provider := NewProvider(endpoints)

	instances, err := provider.Discover(context.Background(), "api")
	if err != nil {
		t.Fatalf("Discover api: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Discover api returned %d instances, want 2", len(instances))
	}

	cases := []struct {
		name         string
		inst         discovery.ServiceInstance
		wantWeight   int
		wantHealth   discovery.HealthStatus
		wantTags     []string
		wantMetadata map[string]string
	}{
		{
			name:         "default weight and mirrored protocol",
			inst:         instances[0],
			wantWeight:   1,
			wantHealth:   discovery.HealthHealthy,
			wantTags:     []string{"grpc"},
			wantMetadata: map[string]string{"protocol": "grpc"},
		},
		{
			name:         "preserves explicit tags metadata weight and health",
			inst:         instances[1],
			wantWeight:   5,
			wantHealth:   discovery.HealthUnhealthy,
			wantTags:     []string{"custom"},
			wantMetadata: map[string]string{"protocol": "custom"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.inst.Weight != tc.wantWeight {
				t.Fatalf("Weight = %d, want %d", tc.inst.Weight, tc.wantWeight)
			}
			if tc.inst.Health != tc.wantHealth {
				t.Fatalf("Health = %q, want %q", tc.inst.Health, tc.wantHealth)
			}
			if len(tc.inst.Tags) != len(tc.wantTags) || tc.inst.Tags[0] != tc.wantTags[0] {
				t.Fatalf("Tags = %v, want %v", tc.inst.Tags, tc.wantTags)
			}
			if tc.inst.Metadata["protocol"] != tc.wantMetadata["protocol"] {
				t.Fatalf("Metadata[protocol] = %q, want %q", tc.inst.Metadata["protocol"], tc.wantMetadata["protocol"])
			}
		})
	}

	emptyInstances, err := provider.Discover(context.Background(), "empty")
	if err != nil {
		t.Fatalf("Discover empty: %v", err)
	}
	if emptyInstances[0].Health != discovery.HealthHealthy {
		t.Fatalf("empty address endpoint health = %q, want healthy", emptyInstances[0].Health)
	}
}

func TestProviderRegisterDeregisterDiscoverStats(t *testing.T) {
	t.Parallel()

	provider := NewProvider(nil)
	ctx := context.Background()
	svc := &discovery.ServiceInfo{
		ID:       "api-1",
		Name:     "api",
		Address:  "127.0.0.1",
		Port:     8080,
		Tags:     []string{"grpc"},
		Metadata: map[string]string{"version": "1"},
	}

	if err := provider.Register(ctx, svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := provider.Stats().RegisteredServices; got != 1 {
		t.Fatalf("RegisteredServices = %d, want 1", got)
	}

	instances, err := provider.Discover(ctx, "api")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != svc.ID || instances[0].Health != discovery.HealthHealthy {
		t.Fatalf("Discover returned %+v, want registered healthy instance", instances)
	}
	if instances[0].LastSeen.IsZero() {
		t.Fatal("LastSeen was not set")
	}

	if err := provider.UpdateHealth(ctx, svc.ID, false, "ignored"); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}
	if err := provider.Deregister(ctx, svc.ID); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if got := provider.Stats().RegisteredServices; got != 0 {
		t.Fatalf("RegisteredServices after deregister = %d, want 0", got)
	}
	if _, err := provider.Discover(ctx, "api"); !errors.Is(err, discovery.ErrServiceNotFound) {
		t.Fatalf("Discover unknown error = %v, want ErrServiceNotFound", err)
	}
	if err := provider.Deregister(ctx, "missing"); err != nil {
		t.Fatalf("Deregister missing: %v", err)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestProviderWatchClosesOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch, err := NewProvider(nil).Watch(ctx, "api")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("watch channel is open, want closed")
		}
	case <-time.After(time.Second):
		t.Fatal("watch channel did not close after context cancellation")
	}
}

func TestRegisterProviders(t *testing.T) {
	t.Parallel()

	reg := discovery.NewProviderRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	for _, name := range []string{"static", "k8s"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			factory, ok := reg.Get(name)
			if !ok {
				t.Fatalf("provider %q not registered", name)
			}
			registry, disc, err := factory(discovery.Config{StaticEndpoints: []discovery.StaticEndpoint{{Name: "api", Address: "127.0.0.1", Port: 8080, Healthy: true}}}, logging.NewDefault("test"))
			if err != nil {
				t.Fatalf("factory %q: %v", name, err)
			}
			if registry == nil || disc == nil {
				t.Fatalf("factory %q returned nil registry/discovery", name)
			}
		})
	}

	if err := Register(reg); err == nil {
		t.Fatal("second Register succeeded, want duplicate-name error")
	}
}
