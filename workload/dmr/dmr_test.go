package dmr_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/workload"
	"github.com/kbukum/gokit/workload/dmr"
)

// fakeDMR is an in-process stand-in for the Docker Model Runner daemon. It
// records requests and serves canned responses so tests never touch a real
// daemon or the network.
type fakeDMR struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest
	models   map[string]int64 // ref -> size bytes
	running  map[string]bool  // ref -> loaded runner present
	created  int64            // created timestamp reported for models

	pullStatus    int    // pre-stream status for POST /models/create (0 => stream 200)
	pullStreamErr string // plain-text terminal error appended mid-stream (after 200)
	pullErrEvent  string // JSON error event emitted mid-stream (after 200)
	listStatus    int    // status returned by GET /models (0 => 200)
	configStatus  int    // status returned by POST /engines/_configure (0 => 202)
	unloadStatus  int    // status returned by POST /engines/unload (0 => 200)
}

type recordedRequest struct {
	method string
	path   string
	body   string
}

func newFakeDMR(t *testing.T) *fakeDMR {
	t.Helper()
	f := &fakeDMR{models: make(map[string]int64), running: make(map[string]bool)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeDMR) record(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recordedRequest{method: r.Method, path: r.URL.Path, body: string(body)})
	return string(body)
}

func (f *fakeDMR) handle(w http.ResponseWriter, r *http.Request) {
	body := f.record(r)
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/models/create":
		var req struct {
			From string `json:"from"`
		}
		_ = json.Unmarshal([]byte(body), &req)
		// Pre-stream failure: DMR reports early errors (bad ref, unauthorized,
		// not found) via status code before any progress byte.
		if f.pullStatus != 0 {
			http.Error(w, "pull failed", f.pullStatus)
			return
		}
		// Real DMR streams chunked JSON progress and only finishes the response
		// when the pull completes.
		w.Header().Set("Content-Type", "application/json")
		flusher, _ := w.(http.Flusher)
		writeChunk := func(s string) {
			_, _ = io.WriteString(w, s+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		writeChunk(`{"type":"progress","message":"pulling"}`)
		switch {
		case f.pullErrEvent != "":
			// Real DMR failure: an in-band {"type":"error"} event. DMR then closes
			// the stream (the handler's http.Error tail is a no-op post-200).
			writeChunk(`{"type":"error","message":"` + f.pullErrEvent + `"}`)
		case f.pullStreamErr != "":
			// Mimic http.Error after the 200 header is committed: plain text.
			writeChunk(f.pullStreamErr)
		default:
			f.mu.Lock()
			f.models[req.From] = 1024
			f.mu.Unlock()
			writeChunk(`{"type":"success","message":"Model pulled successfully"}`)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/engines/_configure":
		if f.configStatus != 0 {
			http.Error(w, "configure failed", f.configStatus)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal([]byte(body), &req)
		f.mu.Lock()
		f.running[req.Model] = true
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodPost && r.URL.Path == "/engines/unload":
		if f.unloadStatus != 0 {
			http.Error(w, "unload failed", f.unloadStatus)
			return
		}
		var req struct {
			Models []string `json:"models"`
		}
		_ = json.Unmarshal([]byte(body), &req)
		n := 0
		f.mu.Lock()
		for _, m := range req.Models {
			if f.running[m] {
				delete(f.running, m)
				n++
			}
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]int{"unloaded_runners": n})
	case r.Method == http.MethodGet && r.URL.Path == "/engines/ps":
		f.mu.Lock()
		ps := make([]map[string]any, 0, len(f.running))
		for ref := range f.running {
			ps = append(ps, map[string]any{"backend_name": "llama.cpp", "model_name": ref})
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(ps)
	case r.Method == http.MethodGet && r.URL.Path == "/models":
		if f.listStatus != 0 {
			http.Error(w, "list failed", f.listStatus)
			return
		}
		f.writeModelList(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/models/"):
		ref := strings.TrimPrefix(r.URL.Path, "/models/")
		f.mu.Lock()
		size, ok := f.models[ref]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(nativeModel(ref, size, f.created))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/models/"):
		ref := strings.TrimPrefix(r.URL.Path, "/models/")
		f.mu.Lock()
		_, ok := f.models[ref]
		delete(f.models, ref)
		f.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusTeapot)
	}
}

func (f *fakeDMR) writeModelList(w http.ResponseWriter) {
	f.mu.Lock()
	list := make([]map[string]any, 0, len(f.models))
	for ref, size := range f.models {
		list = append(list, nativeModel(ref, size, f.created))
	}
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(list)
}

// nativeModel builds a DMR native Model record: the tag lives in tags, the ID is
// a content digest, and on-disk size is a byte-count string nested under config.
func nativeModel(ref string, size, created int64) map[string]any {
	return map[string]any{
		"id":      "sha256:deadbeef",
		"tags":    []string{ref},
		"created": created,
		"config":  map[string]any{"size": strconv.FormatInt(size, 10)},
	}
}

func (f *fakeDMR) lastRequest(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no requests recorded")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeDMR) sawRequest(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.method == method && r.path == path {
			return true
		}
	}
	return false
}

func newRuntime(t *testing.T, base string) workload.ModelRuntime {
	t.Helper()
	reg := workload.NewModelRuntimeRegistry()
	if err := dmr.Register(reg, dmr.Config{BaseURL: base}); err != nil {
		t.Fatalf("register: %v", err)
	}
	rt, err := workload.NewModelRuntime(reg, workload.ModelRuntimeConfig{Provider: workload.ProviderDMR}, testLogger())
	if err != nil {
		t.Fatalf("NewModelRuntime: %v", err)
	}
	return rt
}

// health invokes the runtime's optional [workload.ModelHealthChecker] capability.
func health(t *testing.T, rt workload.ModelRuntime, ctx context.Context) error {
	t.Helper()
	hc, ok := rt.(workload.ModelHealthChecker)
	if !ok {
		t.Fatal("dmr runtime should implement ModelHealthChecker")
	}
	return hc.Health(ctx)
}

// stats invokes the runtime's optional [workload.ModelStatsReporter] capability.
func stats(t *testing.T, rt workload.ModelRuntime, ctx context.Context, model string) (*workload.ModelStats, error) {
	t.Helper()
	sr, ok := rt.(workload.ModelStatsReporter)
	if !ok {
		t.Fatal("dmr runtime should implement ModelStatsReporter")
	}
	return sr.Stats(ctx, model)
}

func TestStartPullsAndReturnsEndpoint(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	h, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status != workload.StatusRunning || h.Model != "ai/smollm2" {
		t.Fatalf("unexpected handle: %+v", h)
	}
	if !strings.HasSuffix(h.Endpoint.BaseURL, "/engines/v1") || h.Endpoint.API != workload.APIOpenAI {
		t.Fatalf("unexpected endpoint: %+v", h.Endpoint)
	}

	req := f.lastRequest(t)
	if req.method != http.MethodPost || req.path != "/models/create" {
		t.Fatalf("expected POST /models/create, got %s %s", req.method, req.path)
	}
	if !strings.Contains(req.body, `"from":"ai/smollm2"`) {
		t.Fatalf("expected from field in body, got %q", req.body)
	}
}

func TestStartPullFailurePreservesCause(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	f.pullStatus = http.StatusInternalServerError
	rt := newRuntime(t, f.srv.URL)

	_, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
	if err == nil {
		t.Fatal("expected pull failure error")
	}
}

// A failure after the 200 header is committed arrives as plain text in the
// streamed body, not via the status code. Start must surface it, not report success.
func TestStartMidStreamPlainTextFailure(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	f.pullStreamErr = "disk full while pulling"
	rt := newRuntime(t, f.srv.URL)

	_, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected mid-stream error surfaced, got %v", err)
	}
}

// A failure reported as a JSON error event in the progress stream must also fail Start.
func TestStartMidStreamErrorEvent(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	f.pullErrEvent = "model not found"
	rt := newRuntime(t, f.srv.URL)

	_, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected error-event surfaced, got %v", err)
	}
}

func TestStartRequiresRef(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	if _, err := rt.Start(context.Background(), workload.ModelSpec{}); err == nil {
		t.Fatal("expected error for empty ref")
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	if err := health(t, rt, context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}

	f.listStatus = http.StatusServiceUnavailable
	if err := health(t, rt, context.Background()); err == nil {
		t.Fatal("expected health failure")
	}
}

func TestEndpoint(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	ep, err := rt.Endpoint(context.Background(), "ai/smollm2")
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if !strings.HasSuffix(ep.BaseURL, "/engines/v1") || ep.Model != "ai/smollm2" || ep.API != workload.APIOpenAI {
		t.Fatalf("unexpected endpoint: %+v", ep)
	}
}

func TestStatsReportsDiskSizeAndLoaded(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Not loaded until a runner is running.
	st, err := stats(t, rt, context.Background(), "ai/smollm2")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.DiskSizeBytes != 1024 {
		t.Fatalf("expected disk size 1024, got %d", st.DiskSizeBytes)
	}
	if st.Loaded {
		t.Fatal("expected model to report not loaded before a runner is active")
	}

	f.mu.Lock()
	f.running["ai/smollm2"] = true
	f.mu.Unlock()
	st, err = stats(t, rt, context.Background(), "ai/smollm2")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if !st.Loaded {
		t.Fatal("expected model to report loaded once a runner is active")
	}
}

func TestStatsRequiresModel(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	if _, err := stats(t, rt, context.Background(), ""); err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestStatsNotFoundReturnsTypedError(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	_, err := stats(t, rt, context.Background(), "ai/missing")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	var apiErr *dmr.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("expected typed 404 dmr.Error, got %v", err)
	}
}

func TestListAndRemoveModels(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	lister, ok := rt.(workload.ModelLister)
	if !ok {
		t.Fatal("dmr runtime should implement ModelLister")
	}
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Ref != "ai/smollm2" {
		t.Fatalf("unexpected models: %+v", models)
	}

	remover, ok := rt.(workload.ModelRemover)
	if !ok {
		t.Fatal("dmr runtime should implement ModelRemover")
	}
	if err := remover.RemoveModel(context.Background(), "ai/smollm2"); err != nil {
		t.Fatalf("RemoveModel: %v", err)
	}
	models, _ = lister.ListModels(context.Background())
	if len(models) != 0 {
		t.Fatalf("expected 0 models, got %d", len(models))
	}
}

func TestStopUnloadsModel(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	f.mu.Lock()
	f.running["ai/smollm2"] = true
	f.mu.Unlock()

	if err := rt.Stop(context.Background(), "ai/smollm2"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	req := f.lastRequest(t)
	if req.method != http.MethodPost || req.path != "/engines/unload" {
		t.Fatalf("expected POST /engines/unload, got %s %s", req.method, req.path)
	}
	if !strings.Contains(req.body, `"ai/smollm2"`) {
		t.Fatalf("expected model in unload body, got %q", req.body)
	}
	f.mu.Lock()
	stillRunning := f.running["ai/smollm2"]
	f.mu.Unlock()
	if stillRunning {
		t.Fatal("expected model to be unloaded")
	}
}

func TestStopIsIdempotentForUnknownModel(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	if err := rt.Stop(context.Background(), "ai/never-loaded"); err != nil {
		t.Fatalf("Stop of unknown model should be a no-op, got %v", err)
	}
}

func TestStopRequiresModel(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	if err := rt.Stop(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestStartAppliesContextSize(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2", ContextSize: 4096}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	req := f.lastRequest(t)
	if req.method != http.MethodPost || req.path != "/engines/_configure" {
		t.Fatalf("expected POST /engines/_configure, got %s %s", req.method, req.path)
	}
	if !strings.Contains(req.body, `"context-size":4096`) || !strings.Contains(req.body, `"model":"ai/smollm2"`) {
		t.Fatalf("expected model and context-size in configure body, got %q", req.body)
	}
}

func TestStartWithoutContextSizeSkipsConfigure(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	req := f.lastRequest(t)
	if req.path == "/engines/_configure" {
		t.Fatal("configure should not be called when ContextSize is zero")
	}
}

func TestStartConfigureFailurePropagates(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	f.configStatus = http.StatusConflict
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2", ContextSize: 4096}); err == nil {
		t.Fatal("expected configure failure to fail Start")
	}
}

func TestStartRejectsResources(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	_, err := rt.Start(context.Background(), workload.ModelSpec{
		Ref:       "ai/smollm2",
		Resources: &workload.ResourceConfig{MemoryLimit: "512m"},
	})
	if err == nil || !strings.Contains(err.Error(), "Resources") {
		t.Fatalf("expected Resources rejection, got %v", err)
	}
}

func TestListReportsCreated(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	f.created = 1700000000
	rt := newRuntime(t, f.srv.URL)

	if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	models, err := rt.(workload.ModelLister).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Created.Unix() != 1700000000 {
		t.Fatalf("expected created timestamp mapped, got %+v", models)
	}
}

func TestModelRefWithSpecialCharsIsEscaped(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	// A ref containing a space and '#' must not corrupt the request path; the
	// fake decodes r.URL.Path back to the original ref.
	ref := "ai/smol #lm2"
	f.mu.Lock()
	f.models[ref] = 2048
	f.mu.Unlock()

	st, err := stats(t, rt, context.Background(), ref)
	if err != nil {
		t.Fatalf("Stats with special-char ref: %v", err)
	}
	if st.DiskSizeBytes != 2048 {
		t.Fatalf("expected disk size 2048, got %d", st.DiskSizeBytes)
	}
	if !f.sawRequest(http.MethodGet, "/models/"+ref) {
		t.Fatalf("expected a GET for decoded path %q; requests: %+v", "/models/"+ref, f.requests)
	}
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := health(t, rt, ctx); err == nil {
		t.Fatal("expected canceled context to fail health")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	// A nil registry is rejected by Register.
	if err := dmr.Register(nil, dmr.Config{}); err == nil {
		t.Fatal("expected error for nil registry")
	}
	// An invalid config is rejected at registration, before any factory runs.
	reg := workload.NewModelRuntimeRegistry()
	if err := dmr.Register(reg, dmr.Config{BaseURL: "ftp://localhost"}); err == nil {
		t.Fatal("expected error for invalid base_url at registration")
	}
}

func TestConfigValidateRejectsBadURL(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]*dmr.Config{
		"bad-scheme":    {BaseURL: "ftp://localhost:12434"},
		"no-host":       {BaseURL: "http://"},
		"with-query":    {BaseURL: "http://localhost:12434?x=1"},
		"with-fragment": {BaseURL: "http://localhost:12434#frag"},
		"with-userinfo": {BaseURL: "http://user:pass@localhost:12434"},
	} {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
	c := &dmr.Config{}
	c.ApplyDefaults()
	if c.BaseURL != dmr.DefaultBaseURL {
		t.Fatalf("ApplyDefaults did not set default base URL, got %q", c.BaseURL)
	}
	if c.Timeout != dmr.DefaultTimeout {
		t.Fatalf("ApplyDefaults did not set default timeout, got %v", c.Timeout)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
}

func TestEndpointRequiresModel(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	if _, err := rt.Endpoint(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestRemoveModelNotFound(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)
	remover := rt.(workload.ModelRemover)
	if err := remover.RemoveModel(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty model")
	}
	if err := remover.RemoveModel(context.Background(), "ai/missing"); err == nil {
		t.Fatal("expected error removing unknown model")
	}
}

func TestTransportErrorSurfaces(t *testing.T) {
	t.Parallel()
	// A closed server yields a deterministic, hermetic connection-refused error
	// (no reliance on a host port being unused).
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	rt, err := dmr.NewRuntime(&dmr.Config{BaseURL: base}, testLogger())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := health(t, rt, context.Background()); err == nil {
		t.Fatal("expected health failure against unreachable daemon")
	}
}

// A negative or over-range context size is rejected before any pull happens.
func TestStartRejectsOutOfRangeContextSize(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	sizes := map[string]int{"negative": -1}
	// math.MaxInt32+1 is not representable by int on 32-bit targets, so derive
	// the over-range case from a runtime int64 and only exercise it where int is
	// wide enough to hold it.
	if strconv.IntSize > 32 {
		sizes["overrange"] = int(int64(math.MaxInt32) + 1)
	}
	for name, size := range sizes {
		if _, err := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2", ContextSize: size}); err == nil {
			t.Fatalf("%s: expected out-of-range context size to be rejected", name)
		}
	}
	// No pull side effect should have occurred for the rejected inputs.
	if f.sawRequest(http.MethodPost, "/models/create") {
		t.Fatal("expected no pull for out-of-range context size")
	}
}

// A pull made with a deadline-less context must be bounded by the configured
// PullTimeout so a stalled create stream cannot hang forever.
func TestPullAppliesDeadlineWhenContextHasNone(t *testing.T) {
	t.Parallel()
	// release lets the stalled handler return during teardown so Close never
	// blocks on a leaked connection, independent of client-side cancellation.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models/create" {
			select {
			case <-r.Context().Done(): // the applied deadline cancels the request
			case <-release:
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Cleanup is LIFO: unblock the handler before the server waits on Close.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	rt, err := dmr.NewRuntime(&dmr.Config{BaseURL: srv.URL, PullTimeout: 50 * time.Millisecond}, testLogger())
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := rt.Start(context.Background(), workload.ModelSpec{Ref: "ai/smollm2"})
		done <- e
	}()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("expected pull to fail under the applied deadline")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pull did not honor the applied PullTimeout deadline")
	}
}

// An untrusted model reference with dot segments must never escape the /models/
// route.
func TestModelRefTraversalRejected(t *testing.T) {
	t.Parallel()
	f := newFakeDMR(t)
	rt := newRuntime(t, f.srv.URL)

	for _, ref := range []string{"../engines/ps", "ai/../../secret", "a/./b"} {
		if _, err := stats(t, rt, context.Background(), ref); err == nil {
			t.Fatalf("expected traversal ref %q to be rejected", ref)
		}
		remover := rt.(workload.ModelRemover)
		if err := remover.RemoveModel(context.Background(), ref); err == nil {
			t.Fatalf("expected traversal ref %q to be rejected on remove", ref)
		}
	}
}

func testLogger() *logging.Logger {
	return logging.MustNew(&logging.Config{Level: "error", Format: "json"}, "dmr-test")
}
