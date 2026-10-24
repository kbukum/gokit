package openaicompat_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/workload"
	"github.com/kbukum/gokit/workload/openaicompat"
)

// Recorded shapes: vLLM's ModelCard adds max_model_len to OpenAI's {id, object, created, owned_by}.
const recorded = `{"object":"list","data":[
 {"id":"Qwen/Qwen3-8B","object":"model","created":1747000000,"owned_by":"vllm","root":"Qwen/Qwen3-8B","parent":null,"max_model_len":32768},
 {"id":"gpt-4o-mini","object":"model","created":1721172741,"owned_by":"system"},
 {"id":"","object":"model"}
]}`

func serve(t *testing.T, status int, body string, auth *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth != nil {
			*auth = r.Header.Get("Authorization")
		}
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func list(t *testing.T, cfg openaicompat.Config) ([]workload.ModelInfo, error) {
	t.Helper()
	l, err := openaicompat.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l.ListModels(context.Background())
}

func TestListDescribesWhatTheServerStates(t *testing.T) {
	t.Parallel()
	var auth string
	srv := serve(t, http.StatusOK, recorded, &auth)
	models, err := list(t, openaicompat.Config{BaseURL: srv.URL + "/v1/", Auth: httpclient.BearerAuth("k")})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer k" || len(models) != 2 {
		t.Fatalf("auth %q models %+v", auth, models)
	}
	v := models[0]
	if v.Ref != "Qwen/Qwen3-8B" || v.Created.Unix() != 1747000000 || v.Model.Provider != ai.ProviderOpenAICompat {
		t.Fatalf("vllm: %+v", v)
	}
	if v.Model.Capabilities.ContextLength != ai.Stated(32768, ai.SourceRuntime) || v.Model.Family.Known() || v.Model.Task.Known() {
		t.Fatalf("vllm descriptor: %+v", v.Model)
	}
	if models[1].Model.Capabilities.ContextLength.Known() {
		t.Fatalf("plain OpenAI states no context: %+v", models[1].Model)
	}
}

func TestErrorsAndMalformedBodies(t *testing.T) {
	t.Parallel()
	srv := serve(t, http.StatusForbidden, `{"error":{"message":"key sk-abc"}}`, nil)
	_, err := list(t, openaicompat.Config{BaseURL: srv.URL + "/v1"})
	var apiErr *openaicompat.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden || strings.Contains(err.Error(), "sk-abc") {
		t.Fatalf("got %v", err)
	}
	bad := serve(t, http.StatusOK, `{"object":"list"}`, nil)
	if _, err := list(t, openaicompat.Config{BaseURL: bad.URL + "/v1"}); err == nil {
		t.Fatal("a body without data must be refused")
	}
	if _, err := openaicompat.New(openaicompat.Config{BaseURL: "http://x/v1?key=1"}); err == nil {
		t.Fatal("a query must be refused")
	}
}
