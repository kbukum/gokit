package kubernetes

import (
	"errors"
	"fmt"
)

// Config holds Kubernetes-specific workload configuration.
type Config struct {
	// Kubeconfig is the path to the kubeconfig file. Empty uses in-cluster config.
	Kubeconfig string `mapstructure:"kubeconfig" json:"kubeconfig"`

	// Context is the kubeconfig context to use. Empty uses the current context.
	Context string `mapstructure:"context" json:"context"`

	// Namespace is the default namespace for workloads. Defaults to "default".
	Namespace string `mapstructure:"namespace" json:"namespace"`

	// ServiceAccount is the default service account for pods.
	ServiceAccount string `mapstructure:"service_account" json:"service_account"`

	// ImagePullPolicy controls when images are pulled: "Always", "IfNotPresent", "Never".
	ImagePullPolicy string `mapstructure:"image_pull_policy" json:"image_pull_policy"`

	// ImagePullSecrets are names of secrets used for private registry auth.
	ImagePullSecrets []string `mapstructure:"image_pull_secrets" json:"image_pull_secrets"`

	// WorkloadType controls what K8s resource is created: "pod", "job". Defaults to "job".
	WorkloadType string `mapstructure:"workload_type" json:"workload_type"`

	// TTLAfterFinished is the seconds after a Job finishes before it's cleaned up. -1 disables.
	TTLAfterFinished *int32 `mapstructure:"ttl_after_finished" json:"ttl_after_finished"`

	// ActiveDeadlineSeconds is the maximum time a Job can run before being terminated.
	ActiveDeadlineSeconds *int64 `mapstructure:"active_deadline_seconds" json:"active_deadline_seconds"`
}

const (
	WorkloadTypePod = "pod"
	WorkloadTypeJob = "job"
)

// ApplyDefaults fills in zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Namespace == "" {
		c.Namespace = "default"
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = "IfNotPresent"
	}
	if c.WorkloadType == "" {
		c.WorkloadType = WorkloadTypeJob
	}
}

// Validate checks the Kubernetes configuration.
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return errors.New("kubernetes: namespace is required")
	}
	switch c.WorkloadType {
	case WorkloadTypePod, WorkloadTypeJob:
	default:
		return fmt.Errorf("kubernetes: unsupported workload_type %q (use %q or %q)", c.WorkloadType, WorkloadTypePod, WorkloadTypeJob)
	}
	switch c.ImagePullPolicy {
	case "Always", "IfNotPresent", "Never":
	default:
		return fmt.Errorf("kubernetes: unsupported image_pull_policy %q", c.ImagePullPolicy)
	}
	return nil
}
