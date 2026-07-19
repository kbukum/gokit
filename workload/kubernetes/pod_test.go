package kubernetes

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
)

func newKubernetesTestManager(workloadType string, objects ...runtime.Object) *Manager {
	return &Manager{
		client:        fake.NewSimpleClientset(objects...),
		cfg:           &Config{Namespace: "default", ImagePullPolicy: "IfNotPresent", WorkloadType: workloadType, ServiceAccount: "default-sa", ImagePullSecrets: []string{"pull-secret"}},
		defaultLabels: map[string]string{"team": "platform", "env": "prod"},
		log:           logging.NewDefault("kubernetes-test"),
	}
}

func richDeployRequest() workload.DeployRequest {
	return workload.DeployRequest{
		Name:           "worker",
		Image:          "example/worker:1",
		Namespace:      "jobs",
		Command:        []string{"worker"},
		Args:           []string{"--once"},
		WorkDir:        "/work",
		Environment:    map[string]string{"A": "1"},
		Labels:         map[string]string{"team": "jobs"},
		Annotations:    map[string]string{"trace": "on"},
		Ports:          []workload.PortMapping{{Container: 8080}, {Container: 5353, Protocol: "udp"}},
		RestartPolicy:  "on-failure",
		ServiceAccount: "request-sa",
		Resources:      &workload.ResourceConfig{CPULimit: "500m", MemoryLimit: "128Mi", CPURequest: "100m", MemoryRequest: "64Mi"},
		Volumes: []workload.VolumeMount{
			{Source: "settings", Target: "/config", Type: "configmap"},
			{Source: "secret", Target: "/secret", Type: "secret", ReadOnly: true},
			{Source: "claim", Target: "/data", Type: "pvc", ReadOnly: true},
			{Target: "/cache", Type: "emptydir"},
			{Source: "/host", Target: "/host"},
		},
		Network: &workload.NetworkConfig{Mode: "host", Hosts: map[string]string{"db": "10.0.0.2"}, DNS: []string{"1.1.1.1"}},
	}
}

func TestDeployJobCreatesJobWithTranslatedPodSpec(t *testing.T) {
	t.Parallel()

	ttl := int32(60)
	deadline := int64(120)
	manager := newKubernetesTestManager(WorkloadTypeJob)
	manager.cfg.TTLAfterFinished = &ttl
	manager.cfg.ActiveDeadlineSeconds = &deadline

	result, err := manager.Deploy(context.Background(), richDeployRequest())
	if err != nil {
		t.Fatalf("deploy job: %v", err)
	}
	if result.ID != "jobs/worker" || result.Status != workload.StatusCreated {
		t.Fatalf("deploy result = %#v", result)
	}
	job, err := manager.client.BatchV1().Jobs("jobs").Get(context.Background(), "worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created job: %v", err)
	}
	if job.Labels["team"] != "jobs" || job.Labels["env"] != "prod" || job.Labels["managed-by"] != "gokit-workload" || job.Annotations["trace"] != "on" {
		t.Fatalf("job metadata = labels:%#v annotations:%#v", job.Labels, job.Annotations)
	}
	if *job.Spec.BackoffLimit != 0 || *job.Spec.TTLSecondsAfterFinished != ttl || *job.Spec.ActiveDeadlineSeconds != deadline {
		t.Fatalf("job policy = %#v", job.Spec)
	}
	assertPodSpecTranslated(t, job.Spec.Template.Spec, corev1.RestartPolicyNever)
}

func TestDeployPodCreatesStandalonePodWithRestartPolicy(t *testing.T) {
	t.Parallel()

	manager := newKubernetesTestManager(WorkloadTypePod)
	result, err := manager.Deploy(context.Background(), richDeployRequest())
	if err != nil {
		t.Fatalf("deploy pod: %v", err)
	}
	if result.ID != "jobs/worker" || result.Status != workload.StatusCreated {
		t.Fatalf("deploy result = %#v", result)
	}
	pod, err := manager.client.CoreV1().Pods("jobs").Get(context.Background(), "worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created pod: %v", err)
	}
	assertPodSpecTranslated(t, pod.Spec, corev1.RestartPolicyOnFailure)
}

func assertPodSpecTranslated(t *testing.T, spec corev1.PodSpec, wantRestart corev1.RestartPolicy) {
	t.Helper()
	if len(spec.Containers) != 1 {
		t.Fatalf("containers = %#v", spec.Containers)
	}
	container := spec.Containers[0]
	if container.Name != "worker" || container.Image != "example/worker:1" || container.ImagePullPolicy != corev1.PullIfNotPresent || container.WorkingDir != "/work" {
		t.Fatalf("container basics = %#v", container)
	}
	if strings.Join(container.Command, " ") != "worker" || strings.Join(container.Args, " ") != "--once" {
		t.Fatalf("command/args = %#v %#v", container.Command, container.Args)
	}
	if len(container.Env) != 1 || container.Env[0].Name != "A" || container.Env[0].Value != "1" {
		t.Fatalf("env = %#v", container.Env)
	}
	if container.Resources.Limits.Cpu().String() != "500m" || container.Resources.Requests.Memory().String() != "64Mi" {
		t.Fatalf("resources = %#v", container.Resources)
	}
	if len(container.Ports) != 2 || container.Ports[0].Protocol != corev1.ProtocolTCP || container.Ports[1].Protocol != corev1.ProtocolUDP {
		t.Fatalf("ports = %#v", container.Ports)
	}
	if len(spec.Volumes) != 5 || spec.Volumes[0].ConfigMap == nil || spec.Volumes[1].Secret == nil || spec.Volumes[2].PersistentVolumeClaim == nil || spec.Volumes[3].EmptyDir == nil || spec.Volumes[4].HostPath == nil {
		t.Fatalf("volumes = %#v", spec.Volumes)
	}
	if len(container.VolumeMounts) != 5 || !container.VolumeMounts[1].ReadOnly || !container.VolumeMounts[2].ReadOnly {
		t.Fatalf("volume mounts = %#v", container.VolumeMounts)
	}
	if spec.ServiceAccountName != "request-sa" || len(spec.ImagePullSecrets) != 1 || spec.ImagePullSecrets[0].Name != "pull-secret" {
		t.Fatalf("service account/secrets = %q %#v", spec.ServiceAccountName, spec.ImagePullSecrets)
	}
	if spec.DNSPolicy != corev1.DNSNone || spec.DNSConfig == nil || len(spec.DNSConfig.Nameservers) == 0 || spec.DNSConfig.Nameservers[0] != "1.1.1.1" || len(spec.HostAliases) != 1 || !spec.HostNetwork {
		t.Fatalf("network = policy:%s dns:%#v aliases:%#v host:%v", spec.DNSPolicy, spec.DNSConfig, spec.HostAliases, spec.HostNetwork)
	}
	if spec.RestartPolicy != wantRestart {
		t.Fatalf("restart policy = %q", spec.RestartPolicy)
	}
}

func TestDeployRejectsUnsupportedRuntimeWorkloadType(t *testing.T) {
	t.Parallel()

	manager := newKubernetesTestManager("deployment")
	_, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "worker", Image: "app"})
	if err == nil || !strings.Contains(err.Error(), "unsupported workload type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestBuildResourceRequirementsIgnoresInvalidQuantities(t *testing.T) {
	t.Parallel()

	reqs := buildResourceRequirements(&workload.ResourceConfig{CPULimit: "bad", MemoryLimit: "also-bad", CPURequest: "100m", MemoryRequest: "64Mi"})
	if len(reqs.Limits) != 0 {
		t.Fatalf("invalid limits should be ignored: %#v", reqs.Limits)
	}
	if reqs.Requests.Cpu().String() != "100m" || reqs.Requests.Memory().String() != "64Mi" {
		t.Fatalf("valid requests should be preserved: %#v", reqs.Requests)
	}
}

func TestDeployPodRestartPolicyDefaultsAndAlways(t *testing.T) {
	t.Parallel()

	manager := newKubernetesTestManager(WorkloadTypePod)
	podSpec := manager.buildPodSpec(workload.DeployRequest{Name: "worker", Image: "app"})
	if podSpec.ServiceAccountName != "default-sa" {
		t.Fatalf("default service account = %q", podSpec.ServiceAccountName)
	}
	result, err := manager.Deploy(context.Background(), workload.DeployRequest{Name: "always", Image: "app", RestartPolicy: "always"})
	if err != nil {
		t.Fatalf("deploy always pod: %v", err)
	}
	pod, err := manager.client.CoreV1().Pods("default").Get(context.Background(), result.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get always pod: %v", err)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Fatalf("restart policy = %q", pod.Spec.RestartPolicy)
	}
}
