package kubernetes

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kbukum/gokit/workload"
)

// deployJob creates a Kubernetes Job.
func (m *Manager) deployJob(ctx context.Context, ns string, req workload.DeployRequest) (*workload.DeployResult, error) {
	podSpec := m.buildPodSpec(req)
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	labels := mergeLabels(m.defaultLabels, req.Labels)
	labels["managed-by"] = "gokit-workload"
	annotations := req.Annotations

	var backoffLimit int32 = 0
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   ns,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &backoffLimit,
			TTLSecondsAfterFinished: m.cfg.TTLAfterFinished,
			ActiveDeadlineSeconds: m.cfg.ActiveDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: podSpec,
			},
		},
	}

	created, err := m.client.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: create job: %w", err)
	}

	m.log.Info("workload deployed as Job", map[string]interface{}{
		"name":      created.Name,
		"namespace": ns,
	})

	return &workload.DeployResult{
		ID:     fmt.Sprintf("%s/%s", ns, created.Name),
		Name:   created.Name,
		Status: workload.StatusCreated,
	}, nil
}

// deployPod creates a standalone Kubernetes Pod.
func (m *Manager) deployPod(ctx context.Context, ns string, req workload.DeployRequest) (*workload.DeployResult, error) {
	podSpec := m.buildPodSpec(req)

	switch req.RestartPolicy {
	case "always":
		podSpec.RestartPolicy = corev1.RestartPolicyAlways
	case "on-failure":
		podSpec.RestartPolicy = corev1.RestartPolicyOnFailure
	default:
		podSpec.RestartPolicy = corev1.RestartPolicyNever
	}

	labels := mergeLabels(m.defaultLabels, req.Labels)
	labels["managed-by"] = "gokit-workload"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   ns,
			Labels:      labels,
			Annotations: req.Annotations,
		},
		Spec: podSpec,
	}

	created, err := m.client.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: create pod: %w", err)
	}

	m.log.Info("workload deployed as Pod", map[string]interface{}{
		"name":      created.Name,
		"namespace": ns,
	})

	return &workload.DeployResult{
		ID:     fmt.Sprintf("%s/%s", ns, created.Name),
		Name:   created.Name,
		Status: workload.StatusCreated,
	}, nil
}

// buildPodSpec creates a PodSpec from a DeployRequest.
func (m *Manager) buildPodSpec(req workload.DeployRequest) corev1.PodSpec {
	container := corev1.Container{
		Name:            req.Name,
		Image:           req.Image,
		ImagePullPolicy: corev1.PullPolicy(m.cfg.ImagePullPolicy),
	}

	if len(req.Command) > 0 {
		container.Command = req.Command
	}
	if len(req.Args) > 0 {
		container.Args = req.Args
	}
	if req.WorkDir != "" {
		container.WorkingDir = req.WorkDir
	}

	// Environment
	for k, v := range req.Environment {
		container.Env = append(container.Env, corev1.EnvVar{Name: k, Value: v})
	}

	// Resources
	if req.Resources != nil {
		container.Resources = buildResourceRequirements(req.Resources)
	}

	// Ports
	for _, p := range req.Ports {
		proto := corev1.ProtocolTCP
		if p.Protocol == "udp" {
			proto = corev1.ProtocolUDP
		}
		container.Ports = append(container.Ports, corev1.ContainerPort{
			ContainerPort: int32(p.Container),
			Protocol:      proto,
		})
	}

	// Volumes
	var volumes []corev1.Volume
	for i, v := range req.Volumes {
		volName := fmt.Sprintf("vol-%d", i)
		mount := corev1.VolumeMount{
			Name:      volName,
			MountPath: v.Target,
			ReadOnly:  v.ReadOnly,
		}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		vol := buildVolume(volName, v)
		volumes = append(volumes, vol)
	}

	spec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		Volumes:    volumes,
	}

	// Service account
	sa := req.ServiceAccount
	if sa == "" {
		sa = m.cfg.ServiceAccount
	}
	if sa != "" {
		spec.ServiceAccountName = sa
	}

	// Image pull secrets
	for _, s := range m.cfg.ImagePullSecrets {
		spec.ImagePullSecrets = append(spec.ImagePullSecrets, corev1.LocalObjectReference{Name: s})
	}

	// DNS
	if req.Network != nil && len(req.Network.DNS) > 0 {
		spec.DNSPolicy = corev1.DNSNone
		spec.DNSConfig = &corev1.PodDNSConfig{
			Nameservers: req.Network.DNS,
		}
	}

	// Host aliases
	if req.Network != nil {
		for host, ip := range req.Network.Hosts {
			spec.HostAliases = append(spec.HostAliases, corev1.HostAlias{
				IP:        ip,
				Hostnames: []string{host},
			})
		}
	}

	// Host network
	if req.Network != nil && req.Network.Mode == "host" {
		spec.HostNetwork = true
	}

	return spec
}

// buildResourceRequirements converts workload ResourceConfig to K8s resource requirements.
func buildResourceRequirements(rc *workload.ResourceConfig) corev1.ResourceRequirements {
	reqs := corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{},
		Requests: corev1.ResourceList{},
	}

	if rc.CPULimit != "" {
		if q, err := resource.ParseQuantity(rc.CPULimit); err == nil {
			reqs.Limits[corev1.ResourceCPU] = q
		}
	}
	if rc.MemoryLimit != "" {
		if q, err := resource.ParseQuantity(rc.MemoryLimit); err == nil {
			reqs.Limits[corev1.ResourceMemory] = q
		}
	}
	if rc.CPURequest != "" {
		if q, err := resource.ParseQuantity(rc.CPURequest); err == nil {
			reqs.Requests[corev1.ResourceCPU] = q
		}
	}
	if rc.MemoryRequest != "" {
		if q, err := resource.ParseQuantity(rc.MemoryRequest); err == nil {
			reqs.Requests[corev1.ResourceMemory] = q
		}
	}
	return reqs
}

// buildVolume creates a K8s Volume from a workload VolumeMount.
func buildVolume(name string, v workload.VolumeMount) corev1.Volume {
	vol := corev1.Volume{Name: name}

	switch v.Type {
	case "configmap":
		vol.VolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: v.Source},
			},
		}
	case "secret":
		vol.VolumeSource = corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: v.Source},
		}
	case "pvc":
		vol.VolumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: v.Source,
				ReadOnly:  v.ReadOnly,
			},
		}
	case "emptydir":
		vol.VolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	default:
		// Default: hostPath bind mount
		hostPathType := corev1.HostPathDirectoryOrCreate
		vol.VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: v.Source,
				Type: &hostPathType,
			},
		}
	}
	return vol
}

// mergeLabels combines default labels with request labels (request wins).
func mergeLabels(defaults, request map[string]string) map[string]string {
	labels := make(map[string]string, len(defaults)+len(request))
	for k, v := range defaults {
		labels[k] = v
	}
	for k, v := range request {
		labels[k] = v
	}
	return labels
}
