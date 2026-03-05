package discovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceResolver_Resolve(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"my-service": {Address: "10.0.0.1", Port: 8080},
		},
	}

	resolver := NewServiceResolver(disc, []string{"my-service"})

	url, err := resolver.Resolve("my-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://10.0.0.1:8080" {
		t.Fatalf("expected http://10.0.0.1:8080, got %s", url)
	}
}

func TestServiceResolver_ResolveWithScheme(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"secure-svc": {Address: "10.0.0.2", Port: 443},
		},
	}

	resolver := NewServiceResolver(disc, []string{"secure-svc"}, WithScheme("https"))

	url, err := resolver.Resolve("secure-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://10.0.0.2:443" {
		t.Fatalf("expected https://10.0.0.2:443, got %s", url)
	}
}

func TestServiceResolver_RejectUnconfiguredService(t *testing.T) {
	disc := &stubDiscoveryClient{}
	resolver := NewServiceResolver(disc, []string{"allowed-svc"})

	_, err := resolver.Resolve("unknown-svc")
	if err == nil {
		t.Fatal("expected error for unconfigured service")
	}
}

func TestServiceResolver_AllowAnyWhenNoServicesConfigured(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"any-svc": {Address: "10.0.0.3", Port: 9090},
		},
	}

	resolver := NewServiceResolver(disc, nil)

	url, err := resolver.Resolve("any-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://10.0.0.3:9090" {
		t.Fatalf("expected http://10.0.0.3:9090, got %s", url)
	}
}

func TestServiceResolver_DiscoveryError(t *testing.T) {
	disc := &stubDiscoveryClient{
		err: errors.New("consul unavailable"),
	}

	resolver := NewServiceResolver(disc, nil)

	_, err := resolver.Resolve("failing-svc")
	if err == nil {
		t.Fatal("expected error when discovery fails")
	}
}

// stubDiscoveryClient is a minimal test double for DiscoveryClient.
type stubDiscoveryClient struct {
	instances map[string]ServiceInstance
	err       error
}

func (s *stubDiscoveryClient) Discover(_ context.Context, serviceName string, _ ...string) ([]ServiceInstance, error) {
	if s.err != nil {
		return nil, s.err
	}
	if inst, ok := s.instances[serviceName]; ok {
		return []ServiceInstance{inst}, nil
	}
	return nil, ErrServiceNotFound
}

func (s *stubDiscoveryClient) DiscoverOne(_ context.Context, q Query) (ServiceInstance, error) {
	if s.err != nil {
		return ServiceInstance{}, s.err
	}
	if inst, ok := s.instances[q.ServiceName]; ok {
		return inst, nil
	}
	return ServiceInstance{}, ErrNoHealthyEndpoints
}

func (s *stubDiscoveryClient) DiscoverAll(_ context.Context) (map[string][]ServiceInstance, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[string][]ServiceInstance)
	for name := range s.instances {
		result[name] = []ServiceInstance{s.instances[name]}
	}
	return result, nil
}

func (s *stubDiscoveryClient) Invalidate(_ string) {}
func (s *stubDiscoveryClient) Close() error        { return nil }

func TestServiceResolver_CustomTimeout(t *testing.T) {
	disc := &stubDiscoveryClient{
		instances: map[string]ServiceInstance{
			"svc": {Address: "10.0.0.4", Port: 3000},
		},
	}

	resolver := NewServiceResolver(disc, nil, WithResolveTimeout(5*time.Second))

	url, err := resolver.Resolve("svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://10.0.0.4:3000" {
		t.Fatalf("expected http://10.0.0.4:3000, got %s", url)
	}
}
