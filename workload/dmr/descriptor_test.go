package dmr_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/workload/dmr"
)

// recordedModels is the shape of docker/model-runner's native GET /models (pkg/distribution/types/config.go).
const recordedModels = `[
 {"id":"sha256:354bf30d0aa3","tags":["ai/smollm2:latest","ai/smollm2"],"created":1742816981,
  "config":{"format":"gguf","quantization":"IQ2_XXS/Q4_K_M","parameters":"361.82 M","architecture":"llama","size":"256.35MiB","context_size":8192,"gguf":{"general.name":"SmolLM2"}}},
 {"id":"sha256:aaaa","tags":["ai/embeddinggemma"],"created":0,"config":{"format":"safetensors","size":"1.2 GB"}}
]`

func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListModelsDescribesEachModel(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusOK, recordedModels)
	rt, err := dmr.NewRuntime(&dmr.Config{BaseURL: srv.URL}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	models, err := rt.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models: %+v", models)
	}
	m := models[0]
	if m.Ref != "ai/smollm2:latest" || m.ID != "sha256:354bf30d0aa3" || m.SizeBytes != 268_802_457 {
		t.Fatalf("info: %+v", m)
	}
	d := m.Model
	if d.Name != "ai/smollm2:latest" || d.Provider != ai.ProviderDMR || d.Digest != "sha256:354bf30d0aa3" {
		t.Fatalf("identity: %+v", d)
	}
	if d.Family != ai.Stated("llama", ai.SourceRuntime) {
		t.Fatalf("family: %+v", d.Family)
	}
	if d.Variant != (ai.Variant{Parameters: "361.82 M", Quantization: "IQ2_XXS/Q4_K_M", Format: "gguf", SizeBytes: 268_802_457}) {
		t.Fatalf("variant: %+v", d.Variant)
	}
	if d.Capabilities.ContextLength != ai.Stated(8192, ai.SourceRuntime) {
		t.Fatalf("context: %+v", d.Capabilities.ContextLength)
	}
	// DMR states no task or capabilities, so they stay unknown.
	if d.Task.Known() || d.Capabilities.Vision.Known() || d.Capabilities.ToolUse.Known() {
		t.Fatalf("unknowns were invented: %+v", d)
	}
	e := models[1].Model
	if e.Family.Known() || e.Capabilities.ContextLength.Known() || e.Variant.SizeBytes != 1_200_000_000 || e.Variant.Format != "safetensors" {
		t.Fatalf("sparse model: %+v", e)
	}
}

func TestParseSize(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int64{
		"": 0, "1024": 1024, "256.35MiB": 268_802_457, "256.35 MiB": 268_802_457, "1.2 GB": 1_200_000_000,
		"3GiB": 3 << 30, "12 kB": 12_000, "2 KiB": 2048, "7 B": 7, "nonsense": 0, "-1 GB": 0, "1 PB": 0,
	} {
		if got := dmr.ParseSize(in); got != want {
			t.Errorf("ParseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestErrorsNeverEchoTheBody(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusUnauthorized, `{"error":"token sk-secret-123 is invalid"}`)
	rt, err := dmr.NewRuntime(&dmr.Config{BaseURL: srv.URL}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.ListModels(context.Background())
	var apiErr *dmr.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected typed 401, got %v", err)
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("error echoes the response body: %v", err)
	}
	for e := error(apiErr); e != nil; e = errors.Unwrap(e) {
		if strings.Contains(e.Error(), "sk-secret") {
			t.Fatalf("wrapped error echoes the body: %v", e)
		}
	}
}

func TestEndpointPolicyRefusesUngrantedHosts(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusOK, recordedModels)
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := dmr.NewRuntime(&dmr.Config{BaseURL: srv.URL, EndpointPolicy: policy}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ListModels(context.Background()); err == nil {
		t.Fatal("a host outside the endpoint policy must be refused")
	}
}
