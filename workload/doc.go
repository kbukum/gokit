// Package workload provides a provider-based workload manager for deploying
// and managing containerized workloads across multiple runtimes.
//
// It follows gokit's component pattern with lifecycle management and supports
// pluggable backends for different container orchestration platforms.
//
// # Backends
//
//   - workload/docker: Docker container management via Docker API
//   - workload/kubernetes: Kubernetes pod management via client-go
//
// # Operations
//
// The workload manager supports full lifecycle operations:
//
//   - Deploy: Create and start workloads
//   - Stop/Remove: Graceful shutdown and cleanup
//   - Logs: Stream container/pod logs
//   - Exec: Execute commands inside running workloads
//   - Events: Monitor workload state changes
package workload
