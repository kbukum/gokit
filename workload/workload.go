// Package workload provides a provider-based workload manager for deploying
// and managing workloads across different runtimes (Docker, Kubernetes, local processes).
package workload

import (
	"context"
	"io"
	"time"
)

// Manager manages workload lifecycle operations.
// All providers must implement this interface.
type Manager interface {
	// Deploy creates and starts a workload, returning its ID.
	Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error)

	// Stop gracefully stops a running workload.
	Stop(ctx context.Context, id string) error

	// Remove removes a stopped workload and cleans up resources.
	Remove(ctx context.Context, id string) error

	// Status returns the current status of a workload.
	Status(ctx context.Context, id string) (*WorkloadStatus, error)

	// Logs returns log output from a workload.
	Logs(ctx context.Context, id string, opts LogOptions) ([]string, error)

	// List returns workloads matching the given filter.
	List(ctx context.Context, filter ListFilter) ([]WorkloadInfo, error)

	// HealthCheck verifies the provider runtime is available.
	HealthCheck(ctx context.Context) error
}

// ExecProvider is optionally implemented by providers that support
// executing commands inside running workloads.
type ExecProvider interface {
	Exec(ctx context.Context, id string, cmd []string) (*ExecResult, error)
}

// StatsProvider is optionally implemented by providers that support
// real-time resource usage statistics.
type StatsProvider interface {
	Stats(ctx context.Context, id string) (*WorkloadStats, error)
}

// LogStreamer is optionally implemented by providers that support
// streaming logs in real-time.
type LogStreamer interface {
	StreamLogs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error)
}

// EventWatcher is optionally implemented by providers that support
// watching workload lifecycle events.
type EventWatcher interface {
	WatchEvents(ctx context.Context, filter ListFilter) (<-chan WorkloadEvent, error)
}

// StatusRunning and other status constants for workload state.
const (
	StatusCreated    = "created"
	StatusRunning    = "running"
	StatusStopped    = "stopped"
	StatusCompleted  = "completed"
	StatusError      = "error"
	StatusRestarting = "restarting"
	StatusUnknown    = "unknown"
	StatusNotFound   = "not_found"
)

// Provider constants for well-known workload runtimes.
const (
	ProviderDocker     = "docker"
	ProviderKubernetes = "kubernetes"
	ProviderProcess    = "process"
)

// DeployRequest describes a workload to deploy.
type DeployRequest struct {
	Name          string            // Human-readable identifier
	Image         string            // Container image or executable path
	Command       []string          // Override entrypoint/command
	Args          []string          // Arguments passed to the command
	Environment   map[string]string // Environment variables
	Labels        map[string]string // Key-value pairs for filtering
	WorkDir       string            // Working directory inside workload
	Resources     *ResourceConfig   // CPU/memory constraints
	Network       *NetworkConfig    // Network configuration
	Volumes       []VolumeMount     // Mount points
	Ports         []PortMapping     // Port mappings
	RestartPolicy string            // "no", "always", "on-failure"
	AutoRemove    bool              // Remove after exit
	Timeout       time.Duration     // Maximum run time (0 = no limit)
	Platform      string            // Target platform (e.g. "linux/amd64")
	Metadata      map[string]any    // Provider-specific extras
}

// DeployResult is returned after a successful deployment.
type DeployResult struct {
	ID     string
	Name   string
	Status string
}

// WorkloadStatus represents the current state of a workload.
type WorkloadStatus struct {
	ID        string
	Name      string
	Image     string
	Status    string
	Running   bool
	Healthy   bool
	StartedAt time.Time
	StoppedAt time.Time
	ExitCode  int
	Message   string
}

// WorkloadInfo contains summary information for list operations.
type WorkloadInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string
	Labels  map[string]string
	Created time.Time
}

// WorkloadStats contains resource usage statistics.
type WorkloadStats struct {
	CPUPercent     float64
	MemoryUsage    int64
	MemoryLimit    int64
	NetworkRxBytes int64
	NetworkTxBytes int64
	PIDs           int
}

// WorkloadEvent represents a lifecycle event.
type WorkloadEvent struct {
	ID        string
	Name      string
	Event     string // "start", "stop", "die", "health_status", "oom"
	Timestamp time.Time
	Message   string
}

// LogOptions controls log retrieval behavior.
type LogOptions struct {
	Tail   int           // Last N lines (0 = all)
	Since  time.Duration // Logs from this duration ago
	Follow bool          // Stream mode (for LogStreamer)
}

// ListFilter filters workloads in List operations.
type ListFilter struct {
	Labels map[string]string // Match ALL labels (AND)
	Name   string            // Name prefix/pattern
	Status string            // Filter by status
}

// ExecResult is returned from ExecProvider.Exec.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ResourceConfig defines compute resource constraints.
type ResourceConfig struct {
	CPULimit      string // "0.5", "1", "500m"
	CPURequest    string // Min CPU (K8s)
	MemoryLimit   string // "512m", "1g"
	MemoryRequest string // Min memory (K8s)
}

// NetworkConfig defines workload networking.
type NetworkConfig struct {
	Mode  string            // Network name, "host", "bridge"
	DNS   []string          // Custom DNS servers
	Hosts map[string]string // Extra /etc/hosts entries
}

// PortMapping maps a workload port to a host port.
type PortMapping struct {
	Host      int
	Container int
	Protocol  string // "tcp" (default), "udp"
}

// VolumeMount defines a storage mount.
type VolumeMount struct {
	Source   string
	Target   string
	ReadOnly bool
}
