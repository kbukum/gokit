package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/workload"
)

func init() {
	workload.RegisterFactory(workload.ProviderKubernetes, func(cfg workload.Config, providerCfg any, log *logger.Logger) (workload.Manager, error) {
		c := &Config{}
		if providerCfg != nil {
			pc, ok := providerCfg.(*Config)
			if !ok {
				return nil, fmt.Errorf("kubernetes: expected *kubernetes.Config, got %T", providerCfg)
			}
			c = pc
		}
		c.ApplyDefaults()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		return NewManager(c, cfg.DefaultLabels, log)
	})
}

// Manager implements workload.Manager using the Kubernetes API.
type Manager struct {
	client        kubernetes.Interface
	cfg           *Config
	defaultLabels map[string]string
	log           *logger.Logger
}

// NewManager creates a new Kubernetes workload manager.
func NewManager(cfg *Config, defaultLabels map[string]string, log *logger.Logger) (*Manager, error) {
	restCfg, err := buildRestConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: create clientset: %w", err)
	}

	return &Manager{
		client:        clientset,
		cfg:           cfg,
		defaultLabels: defaultLabels,
		log:           log,
	}, nil
}

// Deploy creates and starts a Kubernetes workload (Pod or Job).
func (m *Manager) Deploy(ctx context.Context, req workload.DeployRequest) (*workload.DeployResult, error) {
	m.log.Info("deploying workload", map[string]interface{}{
		"name":  req.Name,
		"image": req.Image,
		"type":  m.cfg.WorkloadType,
	})

	ns := m.resolveNamespace(req.Namespace)

	switch m.cfg.WorkloadType {
	case WorkloadTypeJob:
		return m.deployJob(ctx, ns, req)
	case WorkloadTypePod:
		return m.deployPod(ctx, ns, req)
	default:
		return nil, fmt.Errorf("kubernetes: unsupported workload type: %s", m.cfg.WorkloadType)
	}
}

// Stop deletes the workload (Pod or Job).
func (m *Manager) Stop(ctx context.Context, id string) error {
	ns, name := m.parseID(id)
	propagation := metav1.DeletePropagationForeground

	switch m.cfg.WorkloadType {
	case WorkloadTypeJob:
		return m.client.BatchV1().Jobs(ns).Delete(ctx, name, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
	default:
		return m.client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
	}
}

// Remove is equivalent to Stop for Kubernetes (deletes the resource).
func (m *Manager) Remove(ctx context.Context, id string) error {
	return m.Stop(ctx, id)
}

// Restart deletes and recreates the pod. For Jobs, this is not directly supported.
func (m *Manager) Restart(ctx context.Context, id string) error {
	ns, name := m.parseID(id)

	if m.cfg.WorkloadType == WorkloadTypeJob {
		return fmt.Errorf("kubernetes: restart not supported for Jobs — delete and redeploy instead")
	}

	// Get existing pod spec
	pod, err := m.client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("kubernetes: get pod for restart: %w", err)
	}

	// Delete existing
	if err := m.client.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("kubernetes: delete pod for restart: %w", err)
	}

	// Recreate with same spec
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Labels:      pod.Labels,
			Annotations: pod.Annotations,
		},
		Spec: pod.Spec,
	}
	newPod.Spec.NodeName = ""
	newPod.ResourceVersion = ""

	if _, err := m.client.CoreV1().Pods(ns).Create(ctx, newPod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("kubernetes: recreate pod: %w", err)
	}
	return nil
}

// Status returns the current status of a workload.
func (m *Manager) Status(ctx context.Context, id string) (*workload.WorkloadStatus, error) {
	ns, name := m.parseID(id)

	pod, err := m.client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return &workload.WorkloadStatus{
				ID:     id,
				Status: workload.StatusNotFound,
			}, nil
		}
		return nil, fmt.Errorf("kubernetes: get pod: %w", err)
	}

	return podToStatus(pod), nil
}

// Wait blocks until the workload exits.
func (m *Manager) Wait(ctx context.Context, id string) (*workload.WaitResult, error) {
	ns, name := m.parseID(id)

	watcher, err := m.client.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: watch pod: %w", err)
	}
	defer watcher.Stop()

	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			result := &workload.WaitResult{}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Terminated != nil {
					result.StatusCode = int64(cs.State.Terminated.ExitCode)
					if cs.State.Terminated.Reason != "" {
						result.Error = cs.State.Terminated.Reason
					}
					break
				}
			}
			return result, nil
		}
	}
	return nil, ctx.Err()
}

// Logs retrieves log output from a workload.
func (m *Manager) Logs(ctx context.Context, id string, opts workload.LogOptions) ([]string, error) {
	ns, name := m.parseID(id)

	logOpts := &corev1.PodLogOptions{}
	if opts.Tail > 0 {
		tail := int64(opts.Tail)
		logOpts.TailLines = &tail
	}
	if opts.Since > 0 {
		sinceSeconds := int64(opts.Since.Seconds())
		logOpts.SinceSeconds = &sinceSeconds
	}

	req := m.client.CoreV1().Pods(ns).GetLogs(name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: get logs: %w", err)
	}
	defer stream.Close()

	return readLines(stream), nil
}

// List returns workloads matching the given filter.
func (m *Manager) List(ctx context.Context, filter workload.ListFilter) ([]workload.WorkloadInfo, error) {
	ns := filter.Namespace
	if ns == "" {
		ns = m.cfg.Namespace
	}

	labelSelector := buildLabelSelector(filter.Labels, m.defaultLabels)

	pods, err := m.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list pods: %w", err)
	}

	var infos []workload.WorkloadInfo
	for _, pod := range pods.Items {
		if filter.Name != "" && !strings.HasPrefix(pod.Name, filter.Name) {
			continue
		}
		if filter.Status != "" && mapPhase(pod.Status.Phase) != filter.Status {
			continue
		}

		img := ""
		if len(pod.Spec.Containers) > 0 {
			img = pod.Spec.Containers[0].Image
		}
		infos = append(infos, workload.WorkloadInfo{
			ID:        fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
			Name:      pod.Name,
			Image:     img,
			Status:    mapPhase(pod.Status.Phase),
			Labels:    pod.Labels,
			Created:   pod.CreationTimestamp.Time,
			Namespace: pod.Namespace,
		})
	}
	return infos, nil
}

// HealthCheck verifies the Kubernetes cluster is reachable.
func (m *Manager) HealthCheck(ctx context.Context) error {
	_, err := m.client.CoreV1().Namespaces().Get(ctx, m.cfg.Namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("kubernetes: health check failed: %w", err)
	}
	return nil
}

// resolveNamespace returns the request namespace or the config default.
func (m *Manager) resolveNamespace(ns string) string {
	if ns != "" {
		return ns
	}
	return m.cfg.Namespace
}

// parseID splits "namespace/name" into parts. If no slash, uses default namespace.
func (m *Manager) parseID(id string) (namespace, name string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return m.cfg.Namespace, id
}

// podToStatus converts a Kubernetes Pod to WorkloadStatus.
func podToStatus(pod *corev1.Pod) *workload.WorkloadStatus {
	ws := &workload.WorkloadStatus{
		ID:        fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		Name:      pod.Name,
		Status:    mapPhase(pod.Status.Phase),
		Running:   pod.Status.Phase == corev1.PodRunning,
		StartedAt: pod.CreationTimestamp.Time,
	}

	if len(pod.Spec.Containers) > 0 {
		ws.Image = pod.Spec.Containers[0].Image
	}

	// Check readiness
	ws.Ready = true
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			ws.Ready = cond.Status == corev1.ConditionTrue
			break
		}
	}
	ws.Healthy = ws.Ready && ws.Running

	// Restarts and exit code from container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		ws.Restarts += int(cs.RestartCount)
		if cs.State.Terminated != nil {
			ws.ExitCode = int(cs.State.Terminated.ExitCode)
			ws.StoppedAt = cs.State.Terminated.FinishedAt.Time
			ws.Message = cs.State.Terminated.Reason
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Message != "" {
			ws.Message = cs.State.Waiting.Message
		}
	}

	return ws
}

// mapPhase maps Kubernetes pod phase to workload status constants.
func mapPhase(phase corev1.PodPhase) string {
	switch phase {
	case corev1.PodRunning:
		return workload.StatusRunning
	case corev1.PodSucceeded:
		return workload.StatusCompleted
	case corev1.PodFailed:
		return workload.StatusError
	case corev1.PodPending:
		return workload.StatusCreated
	default:
		return workload.StatusUnknown
	}
}

// buildRestConfig creates a Kubernetes REST config from kubeconfig or in-cluster.
func buildRestConfig(cfg *Config) (*rest.Config, error) {
	if cfg.Kubeconfig != "" {
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: cfg.Kubeconfig},
			&clientcmd.ConfigOverrides{CurrentContext: cfg.Context},
		).ClientConfig()
	}
	return rest.InClusterConfig()
}

// buildLabelSelector creates a K8s label selector string from maps.
func buildLabelSelector(labels, defaults map[string]string) string {
	merged := make(map[string]string, len(defaults)+len(labels))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range labels {
		merged[k] = v
	}

	var parts []string
	for k, v := range merged {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

// readLines reads all lines from a reader.
func readLines(r interface{ Read([]byte) (int, error) }) []string {
	var lines []string
	buf := make([]byte, 4096)
	var current strings.Builder

	for {
		n, err := r.Read(buf)
		if n > 0 {
			current.Write(buf[:n])
			// Split accumulated data into lines
			text := current.String()
			for {
				idx := strings.IndexByte(text, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimRight(text[:idx], "\r")
				if line != "" {
					lines = append(lines, line)
				}
				text = text[idx+1:]
			}
			current.Reset()
			current.WriteString(text)
		}
		if err != nil {
			// Flush remaining
			if remaining := current.String(); strings.TrimSpace(remaining) != "" {
				lines = append(lines, remaining)
			}
			break
		}
	}
	return lines
}

// Compile-time interface checks.
var _ workload.Manager = (*Manager)(nil)
