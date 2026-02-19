// Package testutil provides testing utilities for the workload module.
//
// It includes a mock workload manager that implements workload.Manager
// backed by in-memory state, along with a component wrapper.
//
// # Quick Start
//
//	wl := testutil.NewComponent()
//	testutil.T(t).Setup(wl)
//
//	// Deploy a mock workload
//	result, _ := wl.Manager().Deploy(ctx, workload.DeployRequest{Name: "test"})
//
//	// Inspect tracked workloads
//	status, _ := wl.Manager().Status(ctx, result.ID)
package testutil
