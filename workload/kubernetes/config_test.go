package kubernetes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
)

func TestConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.ApplyDefaults()
	if cfg.Namespace != "default" || cfg.ImagePullPolicy != "IfNotPresent" || cfg.WorkloadType != WorkloadTypeJob {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config validates: %v", err)
	}

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "namespace", cfg: Config{ImagePullPolicy: "IfNotPresent", WorkloadType: WorkloadTypeJob}, want: "namespace is required"},
		{name: "workload type", cfg: Config{Namespace: "default", ImagePullPolicy: "IfNotPresent", WorkloadType: "deployment"}, want: "unsupported workload_type"},
		{name: "pull policy", cfg: Config{Namespace: "default", ImagePullPolicy: "Sometimes", WorkloadType: WorkloadTypePod}, want: "unsupported image_pull_policy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRegisterValidatesProviderConfig(t *testing.T) {
	t.Parallel()

	registry := workload.NewFactoryRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("register kubernetes provider: %v", err)
	}
	_, err := workload.New(registry, workload.Config{Provider: workload.ProviderKubernetes}, "not config", logging.NewDefault("test"))
	if err == nil || !strings.Contains(err.Error(), "expected *kubernetes.Config") {
		t.Fatalf("expected typed config error, got %v", err)
	}
}

func TestBuildRestConfigUsesExplicitKubeconfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`)
	if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	restCfg, err := buildRestConfig(&Config{Kubeconfig: path, Context: "test"})
	if err != nil {
		t.Fatalf("build rest config: %v", err)
	}
	if restCfg.Host != "https://127.0.0.1:6443" || restCfg.BearerToken != "test-token" {
		t.Fatalf("rest config = %#v", restCfg)
	}
}
