package lmstudio_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/workload"
	"github.com/kbukum/gokit/workload/lmstudio"
)

// Recorded shape of LM Studio's GET /api/v1/models (https://lmstudio.ai/docs/developer/rest/list).
const recorded = `{"models":[
 {"type":"llm","publisher":"google","key":"google/gemma-3-4b","display_name":"Gemma 3 4B","architecture":"gemma3",
  "quantization":{"name":"Q4_K_M","bits_per_weight":4},"size_bytes":3341081600,"params_string":"4B","loaded_instances":[],
  "max_context_length":131072,"format":"gguf",
  "capabilities":{"vision":true,"trained_for_tool_use":true,"reasoning":{"allowed_options":["off","on"],"default":"off"}}},
 {"type":"llm","key":"qwen/qwen3-1.7b","architecture":"qwen3","quantization":null,"size_bytes":1000,"params_string":null,
  "max_context_length":40960,"format":"mlx","capabilities":{"vision":false,"trained_for_tool_use":false}},
 {"type":"embedding","key":"text-embedding-nomic-embed-text-v1.5","quantization":{"name":"F16","bits_per_weight":16},
  "size_bytes":274290560,"params_string":null,"max_context_length":2048,"format":"gguf"},
 {"type":"decision","key":"lmstudio/decider","max_context_length":null,"format":null},
 {"type":"llm","key":""}
]}`

func serve(t *testing.T, status int, body string, auth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		if r.URL.Path != "/api/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func list(t *testing.T, cfg lmstudio.Config) ([]workload.ModelInfo, error) {
	t.Helper()
	l, err := lmstudio.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l.ListModels(context.Background())
}

func TestListDescribesModels(t *testing.T) {
	t.Parallel()
	var auth string
	srv := serve(t, http.StatusOK, recorded, &auth)
	models, err := list(t, lmstudio.Config{BaseURL: srv.URL, Auth: httpclient.BearerAuth("lm-tok")})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer lm-tok" {
		t.Fatalf("Authorization = %q", auth)
	}
	if len(models) != 4 {
		t.Fatalf("models: %+v", models)
	}
	g := models[0].Model
	if g.Name != "google/gemma-3-4b" || g.Provider != ai.ProviderLMStudio || g.Family != ai.Stated("gemma3", ai.SourceRuntime) {
		t.Fatalf("identity: %+v", g)
	}
	if g.Variant != (ai.Variant{Parameters: "4B", Quantization: "Q4_K_M", Format: "gguf", SizeBytes: 3341081600}) {
		t.Fatalf("variant: %+v", g.Variant)
	}
	c := g.Capabilities
	if !c.Vision.Supported() || !c.ToolUse.Supported() || !c.Reasoning.Supported() || c.ContextLength.Value != 131072 {
		t.Fatalf("caps: %+v", c)
	}
	if g.Task.Value != ai.TaskTextGeneration || len(g.Inputs.Value) != 2 || g.Inputs.Value[1] != ai.ModalityImage {
		t.Fatalf("task/inputs: %+v %+v", g.Task, g.Inputs)
	}
	q := models[1].Model
	if q.Capabilities.Vision.Supported() || !q.Capabilities.Vision.Known() || q.Capabilities.ToolUse.Supported() || q.Capabilities.Reasoning.Known() {
		t.Fatalf("qwen caps (absent reasoning stays unknown): %+v", q.Capabilities)
	}
	if q.Variant.Quantization != "" || len(q.Inputs.Value) != 1 {
		t.Fatalf("qwen: %+v", q)
	}
	e := models[2].Model
	if e.Task.Value != ai.TaskEmbedding || e.Family.Known() || e.Capabilities.Vision.Known() || e.Outputs.Value[0] != ai.ModalityEmbedding {
		t.Fatalf("embedding: %+v", e)
	}
	d := models[3].Model
	if d.Task.Known() || d.Capabilities.ContextLength.Known() {
		t.Fatalf("decision models stay unknown: %+v", d)
	}
}

func TestErrorsAreStatusOnly(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusUnauthorized, `{"error":"bad token lm-secret"}`, nil)
	_, err := list(t, lmstudio.Config{BaseURL: srv.URL})
	var apiErr *lmstudio.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || strings.Contains(err.Error(), "lm-secret") {
		t.Fatalf("got %v", err)
	}
}

func TestMalformedListIsRefused(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"error":"x"}`, `not json`} {
		srv := serve(t, http.StatusOK, body, nil)
		if _, err := list(t, lmstudio.Config{BaseURL: srv.URL}); err == nil {
			t.Fatalf("%q must be refused", body)
		}
	}
}

func TestEndpointPolicyAndConfig(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusOK, recorded, nil)
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list(t, lmstudio.Config{BaseURL: srv.URL, EndpointPolicy: policy}); err == nil {
		t.Fatal("a host outside the policy must be refused")
	}
	if _, err := lmstudio.New(lmstudio.Config{BaseURL: "http://u:p@x"}); err == nil {
		t.Fatal("userinfo must be refused")
	}
}
