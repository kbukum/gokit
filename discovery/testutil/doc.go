// Package testutil provides testing utilities for the discovery module.
//
// It includes a static in-memory discovery/registry test component that
// implements both component.Component and testutil.TestComponent interfaces.
//
// # Quick Start
//
//	disc := testutil.NewComponent()
//	disc.AddInstance("my-service", discovery.ServiceInstance{
//	    ID: "svc-1", Name: "my-service", Address: "127.0.0.1", Port: 8080,
//	    Health: discovery.HealthHealthy,
//	})
//	testutil.T(t).Setup(disc)
//
//	instances, _ := disc.Discovery().Discover(ctx, "my-service")
package testutil
