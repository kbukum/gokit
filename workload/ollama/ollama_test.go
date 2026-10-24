package ollama_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/workload"
	"github.com/kbukum/gokit/workload/ollama"
)

// Recorded shapes of Ollama's GET /api/tags and POST /api/show (api/types.go).
const tags = `{"models":[
 {"name":"llava:7b","model":"llava:7b","modified_at":"2025-05-01T10:00:00Z","size":4733363377,"digest":"8dd30f6b0cb1",
  "details":{"format":"gguf","family":"llama","families":["llama","clip"],"parameter_size":"7B","quantization_level":"Q4_0"}},
 {"name":"nomic-embed-text:latest","model":"nomic-embed-text:latest","size":274302450,"digest":"sha256:0a109f422b47",
  "details":{"format":"gguf","family":"nomic-bert","parameter_size":"137M","quantization_level":"F16"}}
]}`

var shows = map[string]string{
	"llava:7b":                `{"details":{"family":"llama"},"model_info":{"general.architecture":"llama","llama.context_length":32768},"capabilities":["completion","vision"]}`,
	"nomic-embed-text:latest": `{"details":{"family":"nomic-bert"},"model_info":{"general.architecture":"nomic-bert","nomic-bert.context_length":2048},"capabilities":["embedding"]}`,
}

type server struct {
	srv      *httptest.Server
	auth     atomic.Value
	showErr  map[string]int
	showHits atomic.Int32
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{showErr: map[string]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.auth.Store(r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(tags))
		case r.Method == http.MethodPost && r.URL.Path == "/api/show":
			s.showHits.Add(1)
			var req struct{ Model string }
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			if code := s.showErr[req.Model]; code != 0 {
				http.Error(w, `{"error":"secret sk-123"}`, code)
				return
			}
			_, _ = w.Write([]byte(shows[req.Model]))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func list(t *testing.T, cfg ollama.Config) ([]workload.ModelInfo, error) {
	t.Helper()
	l, err := ollama.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var _ workload.ModelLister = l
	return l.ListModels(context.Background())
}

func TestListDescribesModels(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	models, err := list(t, ollama.Config{BaseURL: s.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models: %+v", models)
	}
	v := models[0]
	if v.Ref != "llava:7b" || v.ID != "sha256:8dd30f6b0cb1" || v.SizeBytes != 4733363377 || v.Created.IsZero() {
		t.Fatalf("info: %+v", v)
	}
	d := v.Model
	if d.Name != "llava:7b" || d.Provider != ai.ProviderOllama || d.Digest != "sha256:8dd30f6b0cb1" || d.Family != ai.Stated("llama", ai.SourceRuntime) {
		t.Fatalf("identity: %+v", d)
	}
	if d.Variant != (ai.Variant{Parameters: "7B", Quantization: "Q4_0", Format: "gguf", SizeBytes: 4733363377}) {
		t.Fatalf("variant: %+v", d.Variant)
	}
	c := d.Capabilities
	if c.ContextLength != ai.Stated(32768, ai.SourceRuntime) || !c.Vision.Supported() || c.Vision.Source != ai.SourceRuntime {
		t.Fatalf("caps: %+v", c)
	}
	if !c.ToolUse.Known() || c.ToolUse.Supported() || !c.Reasoning.Known() || c.Reasoning.Supported() {
		t.Fatalf("Ollama's capability list is complete, so absent ones are unsupported: %+v", c)
	}
	if d.Task != ai.Stated(ai.TaskTextGeneration, ai.SourceRuntime) {
		t.Fatalf("task: %+v", d.Task)
	}
	if !d.Inputs.Known() || len(d.Inputs.Value) != 2 || d.Inputs.Value[1] != ai.ModalityImage {
		t.Fatalf("inputs: %+v", d.Inputs)
	}
	e := models[1].Model
	if e.Task != ai.Stated(ai.TaskEmbedding, ai.SourceRuntime) || e.Capabilities.ContextLength.Value != 2048 || e.Digest != "sha256:0a109f422b47" {
		t.Fatalf("embedding: %+v", e)
	}
	if e.Outputs.Value[0] != ai.ModalityEmbedding {
		t.Fatalf("embedding outputs: %+v", e.Outputs)
	}
}

func TestModelRemovedDuringListingIsSkipped(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	s.showErr["llava:7b"] = http.StatusNotFound
	models, err := list(t, ollama.Config{BaseURL: s.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Ref != "nomic-embed-text:latest" {
		t.Fatalf("models: %+v", models)
	}
}

func TestShowFailureDegradesToTagFacts(t *testing.T) {
	t.Parallel()
	for _, code := range []int{http.StatusInternalServerError, http.StatusUnauthorized, http.StatusTooManyRequests} {
		s := newServer(t)
		s.showErr["llava:7b"] = code
		models, err := list(t, ollama.Config{BaseURL: s.srv.URL})
		if err != nil {
			t.Fatalf("%d: one model's show failure must not fail the listing: %v", code, err)
		}
		if len(models) != 2 || models[0].Ref != "llava:7b" || models[1].Ref != "nomic-embed-text:latest" {
			t.Fatalf("%d: models %+v", code, models)
		}
		d := models[0].Model
		if d.Family.Value != "llama" || d.Variant.Parameters != "7B" || d.Capabilities.ContextLength.Known() || d.Capabilities.Vision.Known() || d.Task.Known() {
			t.Fatalf("%d: a degraded model keeps tag facts and leaves the rest unknown: %+v", code, d)
		}
		if !models[1].Model.Capabilities.ContextLength.Known() {
			t.Fatalf("%d: other models are still described: %+v", code, models[1].Model)
		}
	}
}

func TestShowTimeoutDegradesOnlyThatModel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tags))
			return
		}
		var req struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "llava:7b" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(shows[req.Model]))
	}))
	t.Cleanup(srv.Close)
	models, err := list(t, ollama.Config{BaseURL: srv.URL, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Model.Capabilities.ContextLength.Known() || !models[1].Model.Capabilities.ContextLength.Known() {
		t.Fatalf("models: %+v", models)
	}
}

func TestShowsRunConcurrentlyWithinTheBoundAndKeepOrder(t *testing.T) {
	t.Parallel()
	const n, bound = 12, 3
	var tagList strings.Builder
	tagList.WriteString(`{"models":[`)
	for i := range n {
		if i > 0 {
			tagList.WriteString(",")
		}
		fmt.Fprintf(&tagList, `{"name":"m%02d","details":{"family":"llama"}}`, i)
	}
	tagList.WriteString("]}")
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tagList.String()))
			return
		}
		now := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if now <= p || peak.CompareAndSwap(p, now) {
				break
			}
		}
		var req struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		// Later models answer first, so order comes from the listing, not from completion.
		var i int
		_, _ = fmt.Sscanf(req.Model, "m%02d", &i)
		time.Sleep(time.Duration(n-i) * 3 * time.Millisecond)
		_, _ = w.Write([]byte(`{"model_info":{"general.architecture":"llama","llama.context_length":4096},"capabilities":["completion"]}`))
	}))
	t.Cleanup(srv.Close)
	models, err := list(t, ollama.Config{BaseURL: srv.URL, Concurrency: bound})
	if err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got < 2 || got > bound {
		t.Fatalf("peak concurrent show calls %d, want 2..%d", got, bound)
	}
	if len(models) != n {
		t.Fatalf("models: %d", len(models))
	}
	for i, m := range models {
		if m.Ref != fmt.Sprintf("m%02d", i) || !m.Model.Capabilities.ContextLength.Known() {
			t.Fatalf("model %d: %+v", i, m)
		}
	}
}

func TestDeadlineDuringDescribeKeepsTagFacts(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tags))
			return
		}
		var req struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "llava:7b" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte(shows[req.Model]))
	}))
	t.Cleanup(srv.Close)
	l, err := ollama.New(ollama.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	models, err := l.ListModels(ctx)
	if err != nil {
		t.Fatalf("the caller's deadline is a budget for describing, not a failure: %v", err)
	}
	if len(models) != 2 || models[0].Model.Capabilities.ContextLength.Known() || models[0].Model.Family.Value != "llama" ||
		!models[1].Model.Capabilities.ContextLength.Known() {
		t.Fatalf("models: %+v", models)
	}
}

func TestDeadlineDuringTagListFails(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(srv.Close)
	l, err := ollama.New(ollama.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := l.ListModels(ctx); err == nil {
		t.Fatal("a tag list past the deadline listed")
	}
}

func TestCanceledListingFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(tags))
			return
		}
		// Reading the body lets the server notice the client hanging up.
		_, _ = io.Copy(io.Discard, r.Body)
		cancel()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	l, err := ollama.New(ollama.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.ListModels(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("a canceled listing must fail with the cancellation, got %v", err)
	}
}

func TestAllShowsFailingDegradesWithoutEchoingBodies(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	s.showErr["llava:7b"] = http.StatusInternalServerError
	s.showErr["nomic-embed-text:latest"] = http.StatusInternalServerError
	models, err := list(t, ollama.Config{BaseURL: s.srv.URL})
	if err != nil || len(models) != 2 {
		t.Fatalf("models %+v, err %v", models, err)
	}
	for _, m := range models {
		if strings.Contains(fmt.Sprintf("%+v", m), "sk-123") {
			t.Fatalf("a model echoes the error body: %+v", m)
		}
	}
}

func TestBearerCredentialGoesInTheHeader(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	if _, err := list(t, ollama.Config{BaseURL: s.srv.URL, Auth: httpclient.BearerAuth("tok-1")}); err != nil {
		t.Fatal(err)
	}
	if got := s.auth.Load(); got != "Bearer tok-1" {
		t.Fatalf("Authorization = %v", got)
	}
}

func TestShowIsBounded(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	models, err := list(t, ollama.Config{BaseURL: s.srv.URL, MaxDescribed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if s.showHits.Load() != 1 || models[1].Model.Capabilities.ContextLength.Known() || models[1].Model.Family.Value != "nomic-bert" {
		t.Fatalf("show calls %d, second model %+v", s.showHits.Load(), models[1].Model)
	}
}

func TestEndpointPolicyRefusesUngrantedHosts(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	policy, err := security.NewEndpointPolicy([]security.EndpointRule{{Origin: "https://model.example"}}, net.DefaultResolver, &net.Dialer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list(t, ollama.Config{BaseURL: s.srv.URL, EndpointPolicy: policy}); err == nil {
		t.Fatal("a host outside the policy must be refused")
	}
}

func TestConfigIsValidated(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"", "ftp://x", "http://u:p@x", "http://x?q=1", "http://x#f", "http://"} {
		if _, err := ollama.New(ollama.Config{BaseURL: base}); err == nil {
			t.Errorf("base %q must be refused", base)
		}
	}
	if _, err := ollama.New(ollama.Config{BaseURL: "http://x", MaxDescribed: -1}); err == nil {
		t.Error("negative MaxDescribed must be refused")
	}
	for _, c := range []int{-1, ollama.MaxConcurrency + 1} {
		if _, err := ollama.New(ollama.Config{BaseURL: "http://x", Concurrency: c}); err == nil {
			t.Errorf("Concurrency %d must be refused", c)
		}
	}
}

func TestMalformedListIsRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"error":"x"}`)) }))
	t.Cleanup(srv.Close)
	if _, err := list(t, ollama.Config{BaseURL: srv.URL}); err == nil {
		t.Fatal("a success body without models must not read as an empty list")
	}
}
