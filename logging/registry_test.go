package logging

import "testing"

func TestRegistryDerivesAndCaches(t *testing.T) {
	t.Parallel()

	r := NewRegistry(nil) // nil base → usable default
	if r.Base() == nil {
		t.Fatal("Base should never be nil")
	}

	a := r.Get("auth")
	if a == nil {
		t.Fatal("Get should never return nil")
	}
	if r.Get("auth") != a {
		t.Error("Get should cache the derived component logger")
	}

	custom := NewDefault("custom")
	r.Register("auth", custom)
	if r.Get("auth") != custom {
		t.Error("Register should override the derived logger")
	}
}

func TestComponentRegistryTracksComponents(t *testing.T) {
	t.Parallel()

	r := NewComponentRegistry()
	if r.StartTime().IsZero() {
		t.Error("StartTime should be set at construction")
	}

	r.SetAPIPrefix("/api/v1/")
	if r.APIPrefix() != "/api/v1" {
		t.Errorf("APIPrefix = %q, want /api/v1", r.APIPrefix())
	}

	r.RegisterInfrastructure("db", "database", "active", "postgres")
	r.RegisterService("eval", "active", []string{"db"})
	r.RegisterRepository("runs", "PostgreSQL", "active")
	r.RegisterClient("worker", "gRPC:9090", "active")
	r.RegisterHandler("GET", "/health", "health")
	r.RegisterConsumer(ConsumerComponent{Name: "events", Group: "g", Topic: "t", Partitions: 3})

	if len(r.Infrastructure()) != 1 || len(r.Services()) != 1 || len(r.Repositories()) != 1 ||
		len(r.Clients()) != 1 || len(r.Handlers()) != 1 || len(r.Consumers()) != 1 {
		t.Fatalf("expected one of each component registered")
	}

	replacement := []HandlerComponent{{Method: "POST", Path: "/x", Handler: "h"}}
	r.SetHandlers(replacement)
	if got := r.Handlers(); len(got) != 1 || got[0].Method != "POST" {
		t.Errorf("SetHandlers did not replace handlers, got %v", got)
	}
}
