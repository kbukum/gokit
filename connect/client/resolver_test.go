package client

import (
	"errors"
	"testing"
)

func TestResolveFunc(t *testing.T) {
	wantErr := errors.New("discovery failed")
	resolver := ResolveFunc(func(serviceName string) (string, error) {
		if serviceName != "orders" {
			t.Fatalf("serviceName = %q, want orders", serviceName)
		}
		return "", wantErr
	})

	got, err := resolver.Resolve("orders")
	if got != "" {
		t.Fatalf("Resolve returned URL %q, want empty", got)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Resolve error = %v, want %v", err, wantErr)
	}
}

func TestStaticResolver(t *testing.T) {
	resolver := StaticResolver("http://example.test")

	for _, serviceName := range []string{"orders", "payments"} {
		got, err := resolver.Resolve(serviceName)
		if err != nil {
			t.Fatalf("Resolve(%q) returned error: %v", serviceName, err)
		}
		if got != "http://example.test" {
			t.Fatalf("Resolve(%q) = %q, want fixed URL", serviceName, got)
		}
	}
}
