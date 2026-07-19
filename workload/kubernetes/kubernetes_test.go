package kubernetes

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
)

func TestStopRemoveAndRestartUseConfiguredWorkloadType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "jobs", Labels: map[string]string{"app": "worker"}}, Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "worker", Image: "app"}}}}
	podManager := newKubernetesTestManager(WorkloadTypePod, pod)
	if err := podManager.Restart(ctx, "jobs/worker"); err != nil {
		t.Fatalf("restart pod: %v", err)
	}
	recreated, err := podManager.client.CoreV1().Pods("jobs").Get(ctx, "worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get recreated pod: %v", err)
	}
	if recreated.Spec.NodeName != "" || recreated.ResourceVersion != "" || recreated.Labels["app"] != "worker" {
		t.Fatalf("recreated pod = %#v", recreated)
	}
	if err := podManager.Remove(ctx, "jobs/worker"); err != nil {
		t.Fatalf("remove pod: %v", err)
	}

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "jobs"}}
	jobManager := newKubernetesTestManager(WorkloadTypeJob, job)
	if err := jobManager.Restart(ctx, "jobs/job"); err == nil || !strings.Contains(err.Error(), "restart not supported") {
		t.Fatalf("expected job restart error, got %v", err)
	}
	if err := jobManager.Stop(ctx, "jobs/job"); err != nil {
		t.Fatalf("stop job: %v", err)
	}
}

func TestRestartReportsGetDeleteAndCreateFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	missing := newKubernetesTestManager(WorkloadTypePod)
	if err := missing.Restart(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "get pod") {
		t.Fatalf("missing restart error = %v", err)
	}

	deleteFailClient := fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker", Image: "app"}}}})
	deleteFailClient.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("delete failed"))
	})
	deleteFail := &Manager{client: deleteFailClient, cfg: &Config{Namespace: "default", WorkloadType: WorkloadTypePod}, log: logging.NewDefault("test")}
	if err := deleteFail.Restart(ctx, "worker"); err == nil || !strings.Contains(err.Error(), "delete pod") {
		t.Fatalf("delete restart error = %v", err)
	}

	createFailClient := fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker", Image: "app"}}}})
	createFailClient.PrependReactor("create", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("create failed"))
	})
	createFail := &Manager{client: createFailClient, cfg: &Config{Namespace: "default", WorkloadType: WorkloadTypePod}, log: logging.NewDefault("test")}
	if err := createFail.Restart(ctx, "worker"); err == nil || !strings.Contains(err.Error(), "recreate pod") {
		t.Fatalf("create restart error = %v", err)
	}
}

func TestStatusMapsPodStateAndNotFound(t *testing.T) {
	t.Parallel()

	finished := metav1.NewTime(time.Date(2026, 7, 18, 21, 0, 0, 0, time.UTC))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "jobs", CreationTimestamp: metav1.NewTime(time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC))},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "worker", Image: "app:1"}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodFailed,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			ContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 2, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 9, FinishedAt: finished, Reason: "Error"}}},
				{RestartCount: 1, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Message: "backoff"}}},
			},
		},
	}
	manager := newKubernetesTestManager(WorkloadTypePod, pod)
	status, err := manager.Status(context.Background(), "jobs/worker")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.ID != "jobs/worker" || status.Image != "app:1" || status.Status != workload.StatusError || status.Running || status.Ready || status.Healthy || status.Restarts != 3 || status.ExitCode != 9 || status.Message != "backoff" || !status.StoppedAt.Equal(finished.Time) {
		t.Fatalf("status = %#v", status)
	}
	missing, err := manager.Status(context.Background(), "jobs/missing")
	if err != nil {
		t.Fatalf("missing status: %v", err)
	}
	if missing.Status != workload.StatusNotFound {
		t.Fatalf("missing status = %#v", missing)
	}
}

func TestWaitReturnsTerminatedContainerResult(t *testing.T) {
	t.Parallel()

	watcher := watch.NewFake()
	clientset := fake.NewSimpleClientset()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	manager := &Manager{client: clientset, cfg: &Config{Namespace: "default", WorkloadType: WorkloadTypePod}, log: logging.NewDefault("test")}

	resultCh := make(chan *workload.WaitResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := manager.Wait(context.Background(), "worker")
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()
	watcher.Add(&corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded, ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 4, Reason: "Done"}}}}}})

	select {
	case result := <-resultCh:
		if result.StatusCode != 4 || result.Error != "Done" {
			t.Fatalf("wait result = %#v", result)
		}
	case err := <-errCh:
		t.Fatalf("wait error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return after terminal pod event")
	}
}

func TestWaitIgnoresNonPodEventsAndReturnsContextErrorOnClosedWatch(t *testing.T) {
	t.Parallel()

	watcher := watch.NewFake()
	clientset := fake.NewSimpleClientset()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{client: clientset, cfg: &Config{Namespace: "default", WorkloadType: WorkloadTypePod}, log: logging.NewDefault("test")}

	errCh := make(chan error, 1)
	go func() {
		_, err := manager.Wait(ctx, "worker")
		errCh <- err
	}()
	watcher.Add(&corev1.ConfigMap{})
	cancel()
	watcher.Stop()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("wait error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return after closed watch")
	}
}

func TestListFiltersPodsAndBuildsSelectors(t *testing.T) {
	t.Parallel()

	pods := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Namespace: "jobs", Labels: map[string]string{"team": "jobs", "env": "prod"}, CreationTimestamp: metav1.NewTime(time.Unix(1710000000, 0))}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "app:1"}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "jobs", Labels: map[string]string{"team": "jobs", "env": "prod"}}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
	}
	manager := newKubernetesTestManager(WorkloadTypePod, pods...)
	infos, err := manager.List(context.Background(), workload.ListFilter{Namespace: "jobs", Name: "worker", Status: workload.StatusRunning, Labels: map[string]string{"team": "jobs"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "jobs/worker-1" || infos[0].Image != "app:1" || infos[0].Status != workload.StatusRunning || infos[0].Labels["team"] != "jobs" || infos[0].Created.IsZero() {
		t.Fatalf("infos = %#v", infos)
	}
	selector := buildLabelSelector(map[string]string{"team": "jobs"}, map[string]string{"team": "platform", "env": "prod"})
	if !strings.Contains(selector, "team=jobs") || !strings.Contains(selector, "env=prod") || strings.Contains(selector, "team=platform") {
		t.Fatalf("selector = %q", selector)
	}
}

func TestHealthCheckUsesConfiguredNamespace(t *testing.T) {
	t.Parallel()

	manager := newKubernetesTestManager(WorkloadTypePod, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
	if err := manager.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}
	manager.cfg.Namespace = "missing"
	if err := manager.HealthCheck(context.Background()); err == nil || !strings.Contains(err.Error(), "health check failed") {
		t.Fatalf("missing namespace health error = %v", err)
	}
}

func TestResolveNamespaceParseIDMapPhaseAndReadLines(t *testing.T) {
	t.Parallel()

	manager := &Manager{cfg: &Config{Namespace: "default"}}
	if got := manager.resolveNamespace("jobs"); got != "jobs" {
		t.Fatalf("request namespace = %q", got)
	}
	if got := manager.resolveNamespace(""); got != "default" {
		t.Fatalf("default namespace = %q", got)
	}
	if ns, name := manager.parseID("jobs/worker"); ns != "jobs" || name != "worker" {
		t.Fatalf("parse namespaced = %q %q", ns, name)
	}
	if ns, name := manager.parseID("worker"); ns != "default" || name != "worker" {
		t.Fatalf("parse default = %q %q", ns, name)
	}
	phases := map[corev1.PodPhase]string{corev1.PodRunning: workload.StatusRunning, corev1.PodSucceeded: workload.StatusCompleted, corev1.PodFailed: workload.StatusError, corev1.PodPending: workload.StatusCreated, corev1.PodUnknown: workload.StatusUnknown}
	for phase, want := range phases {
		if got := mapPhase(phase); got != want {
			t.Fatalf("mapPhase(%q) = %q", phase, got)
		}
	}
	lines := readLines(strings.NewReader("one\r\ntwo\n\nthree"))
	if strings.Join(lines, ",") != "one,two,three" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestWaitWatchError(t *testing.T) {
	t.Parallel()

	clientset := fake.NewSimpleClientset()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "worker")
	})
	manager := &Manager{client: clientset, cfg: &Config{Namespace: "default", WorkloadType: WorkloadTypePod}, log: logging.NewDefault("test")}
	_, err := manager.Wait(context.Background(), "worker")
	if err == nil || !strings.Contains(err.Error(), "watch pod") {
		t.Fatalf("wait watch error = %v", err)
	}
}

func FuzzReadLines(f *testing.F) {
	f.Add("one\ntwo\n")
	f.Add("one\r\ntwo")
	f.Add("")
	f.Fuzz(func(t *testing.T, input string) {
		lines := readLines(strings.NewReader(input))
		for _, line := range lines {
			if line == "" || strings.Contains(line, "\n") || strings.Contains(line, "\r") {
				t.Fatalf("invalid line %q from %q", line, input)
			}
		}
	})
}

var _ io.Reader = strings.NewReader("")
