// Package component defines the core interfaces for lifecycle-managed
// infrastructure services in gokit.
//
// Components represent services that require initialization, startup,
// shutdown, and health monitoring. They are registered with the bootstrap
// package for automatic lifecycle management.
//
// # Interfaces
//
//   - Component: Core lifecycle interface (Init/Start/Stop)
//   - HealthChecker: Health status reporting
//   - Describable: Bootstrap summary descriptions
package component
