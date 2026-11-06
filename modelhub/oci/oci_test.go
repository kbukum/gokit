package oci_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/gguf"
	"github.com/kbukum/gokit/modelhub/internal/fetchtest"
	"github.com/kbukum/gokit/modelhub/oci"
	"github.com/kbukum/gokit/util"
)

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// registry is a fake distribution registry with anonymous bearer-token auth.
type registry struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	manifests map[string][]byte // "repo:ref" -> body
	types     map[string]string // "repo:ref" -> content type
	blobs     map[string][]byte // digest -> body
	tags      []string
	tokens    atomic.Int32
	challenge atomic.Int32
	token     string
	headerDig string // overrides Docker-Content-Digest when set
	openRoot  bool   // GET /v2/ answers 200 without a token while repositories still require one
	realm     string // overrides the token realm when set
	gets      atomic.Int32
}

func newRegistry(t *testing.T) *registry {
	r := &registry{t: t, manifests: map[string][]byte{}, types: map[string]string{}, blobs: map[string][]byte{}, token: "tok-1"}
	r.srv = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *registry) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.URL.Path == "/token" {
		r.tokens.Add(1)
		if req.URL.Query().Get("service") != "registry.test" || !strings.HasPrefix(req.URL.Query().Get("scope"), "repository:") {
			r.t.Errorf("token request = %s", req.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": r.token, "expires_in": 300})
		return
	}
	if req.URL.Path == "/v2/" && r.openRoot {
		w.WriteHeader(http.StatusOK)
		return
	}
	if req.Header.Get("Authorization") != "Bearer "+r.token {
		if req.URL.Path == "/v2/" {
			r.challenge.Add(1)
		}
		realm := r.srv.URL + "/token"
		if r.realm != "" {
			realm = r.realm
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm=%q,service="registry.test"`, realm))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/v2/"), "/")
	switch {
	case req.URL.Path == "/v2/":
		w.WriteHeader(http.StatusOK)
	case len(parts) >= 3 && parts[len(parts)-2] == "manifests":
		key := strings.Join(parts[:len(parts)-2], "/") + ":" + parts[len(parts)-1]
		if req.Method == http.MethodGet {
			r.gets.Add(1)
		}
		body, ok := r.manifests[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", r.types[key])
		dig := digestOf(body)
		if r.headerDig != "" {
			dig = r.headerDig
		}
		w.Header().Set("Docker-Content-Digest", dig)
		_, _ = w.Write(body)
	case len(parts) >= 3 && parts[len(parts)-2] == "blobs":
		body, ok := r.blobs[parts[len(parts)-1]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if rng := req.Header.Get("Range"); rng != "" {
			end, _ := strconv.Atoi(strings.TrimPrefix(rng, "bytes=0-"))
			if end+1 < len(body) {
				body = body[:end+1]
			}
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(body)
	case len(parts) >= 3 && parts[len(parts)-1] == "list":
		n, _ := strconv.Atoi(req.URL.Query().Get("n"))
		last := req.URL.Query().Get("last")
		start := 0
		for i, tag := range r.tags {
			if tag == last {
				start = i + 1
			}
		}
		end := min(start+n, len(r.tags))
		if end < len(r.tags) {
			repo := strings.Join(parts[:len(parts)-2], "/")
			w.Header().Set("Link", fmt.Sprintf(`</v2/%s/tags/list?n=%d&last=%s>; rel="next"`, repo, n, r.tags[end-1]))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "x", "tags": r.tags[start:end]})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *registry) addBlob(b []byte) string {
	d := digestOf(b)
	r.blobs[d] = b
	return d
}

func (r *registry) addManifest(repo, tag, mediaType string, m any) []byte {
	body, err := json.Marshal(m)
	if err != nil {
		r.t.Fatal(err)
	}
	r.manifests[repo+":"+tag] = body
	r.manifests[repo+":"+digestOf(body)] = body
	r.types[repo+":"+tag] = mediaType
	r.types[repo+":"+digestOf(body)] = mediaType
	return body
}

func desc(mediaType, digest string, size int, annotations map[string]string) map[string]any {
	d := map[string]any{"mediaType": mediaType, "digest": digest, "size": size}
	if annotations != nil {
		d["annotations"] = annotations
	}
	return d
}

func newClient(t *testing.T, r *registry) *oci.Client {
	t.Helper()
	hc := fetchtest.Adapter(t, nil, r.srv.URL)
	c, err := oci.New(oci.Config{HTTP: hc, Registry: r.srv.URL, Clock: util.NewFakeClock(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func dockerModel(r *registry) (weight []byte) {
	cfg := []byte(`{"config":{"format":"gguf","quantization":"Q4_K_M","parameters":"361.82 M","architecture":"llama","size":"256.35 MiB","context_size":8192},"descriptor":{"created":"2025-01-01T00:00:00Z"}}`)
	weight = []byte("GGUF-weights-0123456789")
	license := []byte("Apache License 2.0")
	r.addManifest("ai/smollm2", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config":        desc("application/vnd.docker.ai.model.config.v0.1+json", r.addBlob(cfg), len(cfg), nil),
		"layers": []any{
			desc("application/vnd.docker.ai.gguf.v3", r.addBlob(weight), len(weight), nil),
			desc("application/vnd.docker.ai.license", r.addBlob(license), len(license), nil),
		},
	})
	return weight
}

func TestResolveReadsDockerModelConfig(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	weight := dockerModel(r)
	c := newClient(t, r)
	ref, err := oci.ParseReference("ai/smollm2:latest")
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Resolve(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != oci.KindDocker || m.Format != "gguf" || m.Quantization != "Q4_K_M" || m.Architecture != "llama" || m.Parameters != "361.82 M" || m.ContextLength != 8192 {
		t.Fatalf("model facts = %+v", m)
	}
	if len(m.Weights) != 1 || m.Weights[0].Size != int64(len(weight)) || m.WeightBytes != int64(len(weight)) || len(m.Files) != 1 {
		t.Fatalf("layers = %+v / %+v", m.Weights, m.Files)
	}
	if !strings.HasPrefix(m.Digest, "sha256:") || m.ReadAt.IsZero() || m.Reference.Tag != "latest" {
		t.Fatalf("pin = %q readAt=%v", m.Digest, m.ReadAt)
	}
	// The token is cached: one challenge and one token for the whole resolve.
	if r.challenge.Load() != 1 || r.tokens.Load() != 1 {
		t.Fatalf("challenges=%d tokens=%d", r.challenge.Load(), r.tokens.Load())
	}
	// A pinned read resolves the same digest.
	pinned, err := c.Resolve(t.Context(), oci.Reference{Repository: "ai/smollm2", Digest: m.Digest})
	if err != nil || pinned.Digest != m.Digest {
		t.Fatalf("pinned resolve = %v, %v", pinned, err)
	}
}

func TestResolveReadsModelPackConfig(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	cfg := []byte(`{"descriptor":{"name":"Qwen3","family":"qwen3","licenses":["Apache-2.0"]},"modelfs":{"type":"layers","diffIds":[]},"config":{"format":"gguf","architecture":"transformer","paramSize":"8b","quantization":"Q4_K_M","precision":"int4","capabilities":{"inputTypes":["text"],"outputTypes":["text"],"toolUsage":true,"reasoning":true}}}`)
	weight := []byte("weights")
	readme := []byte("# readme")
	r.addManifest("ai/qwen3", "8B-Q4_K_M", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"artifactType":  "application/vnd.cncf.model.manifest.v1+json",
		"config":        desc("application/vnd.cncf.model.config.v1+json", r.addBlob(cfg), len(cfg), nil),
		"layers": []any{
			desc("application/vnd.cncf.model.weight.v1.raw", r.addBlob(weight), len(weight), map[string]string{"org.cncf.model.filepath": "Qwen3-8B-Q4_K_M.gguf"}),
			desc("application/vnd.cncf.model.doc.v1.raw", r.addBlob(readme), len(readme), map[string]string{"org.cncf.model.filepath": "README.md"}),
		},
	})
	m, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/qwen3", Tag: "8B-Q4_K_M"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != oci.KindModelPack || m.Name != "Qwen3" || m.Family != "qwen3" || m.Parameters != "8b" || m.Precision != "int4" || m.Quantization != "Q4_K_M" {
		t.Fatalf("model facts = %+v", m)
	}
	if len(m.Licenses) != 1 || m.Licenses[0] != "Apache-2.0" || m.Capabilities == nil || m.Capabilities.ToolUsage == nil || !*m.Capabilities.ToolUsage {
		t.Fatalf("licenses/capabilities = %v %+v", m.Licenses, m.Capabilities)
	}
	if len(m.Weights) != 1 || m.Weights[0].Path != "Qwen3-8B-Q4_K_M.gguf" || len(m.Files) != 1 || m.Files[0].Path != "README.md" {
		t.Fatalf("layers = %+v / %+v", m.Weights, m.Files)
	}
}

func TestResolveRejectsTamperedOrHostileManifests(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	dockerModel(r)
	r.headerDig = digestOf([]byte("other"))
	_, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/smollm2", Tag: "latest"})
	if app, ok := apperr.AsAppError(err); !ok || app.Reason != oci.ReasonDigestMismatch {
		t.Fatalf("digest mismatch error = %v", err)
	}

	r2 := newRegistry(t)
	r2.addManifest("ai/index", "latest", "application/vnd.oci.image.index.v1+json", map[string]any{"schemaVersion": 2, "manifests": []any{}})
	cfg := []byte(`{}`)
	r2.addManifest("ai/bigcfg", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2, "config": desc("application/vnd.cncf.model.config.v1+json", r2.addBlob(cfg), 1<<40, nil), "layers": []any{},
	})
	r2.addManifest("ai/baddigest", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2, "config": desc("application/vnd.cncf.model.config.v1+json", "sha256:../../etc", 2, nil), "layers": []any{},
	})
	r2.addManifest("ai/v1", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{"schemaVersion": 1})
	r2.manifests["ai/junk:latest"] = []byte("{not json")
	r2.types["ai/junk:latest"] = "application/vnd.oci.image.manifest.v1+json"
	c := newClient(t, r2)
	for _, repo := range []string{"ai/index", "ai/bigcfg", "ai/baddigest", "ai/v1", "ai/junk"} {
		if _, err := c.Resolve(t.Context(), oci.Reference{Repository: repo, Tag: "latest"}); err == nil {
			t.Errorf("%s: Resolve() accepted a hostile manifest", repo)
		} else if _, ok := apperr.AsAppError(err); !ok {
			t.Errorf("%s: untyped error %v", repo, err)
		}
	}
	if _, err := c.Resolve(t.Context(), oci.Reference{Repository: "ai/missing", Tag: "latest"}); err == nil {
		t.Fatal("Resolve() found a missing repository")
	} else if app, _ := apperr.AsAppError(err); app == nil || app.Code != apperr.ErrCodeNotFound {
		t.Fatalf("missing repository error = %v", err)
	}
}

func TestResolveRejectsConfigDigestMismatch(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	cfg := []byte(`{"config":{"format":"gguf"}}`)
	d := digestOf(cfg)
	r.blobs[d] = []byte(`{"config":{"format":"evil"}}`)
	r.addManifest("ai/tampered", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2, "config": desc("application/vnd.docker.ai.model.config.v0.1+json", d, len(cfg), nil), "layers": []any{},
	})
	_, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/tampered", Tag: "latest"})
	if app, ok := apperr.AsAppError(err); !ok || app.Reason != oci.ReasonDigestMismatch {
		t.Fatalf("config tamper error = %v", err)
	}
}

func TestTagsFollowsPaginationWithinBound(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	for i := range 7 {
		r.tags = append(r.tags, fmt.Sprintf("t%d", i))
	}
	hc := fetchtest.Adapter(t, nil, r.srv.URL)
	c, err := oci.New(oci.Config{HTTP: hc, Registry: r.srv.URL, PageSize: 3, MaxTags: 5})
	if err != nil {
		t.Fatal(err)
	}
	tags, err := c.Tags(t.Context(), "ai/smollm2")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags.Names) != 5 || tags.Names[4] != "t4" || !tags.Truncated {
		t.Fatalf("Tags() = %+v", tags)
	}
	c2, err := oci.New(oci.Config{HTTP: hc, Registry: r.srv.URL, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if all, err := c2.Tags(t.Context(), "ai/smollm2"); err != nil || len(all.Names) != 7 || all.Truncated {
		t.Fatalf("Tags() all = %+v, %v", all, err)
	}
}

func TestBlobFeedsGGUFHeaderReader(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	var header strings.Builder
	header.WriteString("GGUF\x03\x00\x00\x00")
	header.WriteString("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	body := []byte(header.String())
	d := r.addBlob(append(body, make([]byte, 1<<16)...))
	c := newClient(t, r)
	h, err := gguf.Read(t.Context(), c.Blob("ai/smollm2", oci.Layer{Digest: d, Size: int64(len(body)) + 1<<16}), gguf.DefaultLimits())
	if err != nil || h.Version != 3 {
		t.Fatalf("gguf.Read() = %+v, %v", h, err)
	}
	src := c.Blob("ai/smollm2", oci.Layer{Digest: d, Size: 10})
	rc, err := src.OpenPrefix(t.Context(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if len(got) != 10 {
		t.Fatalf("prefix capped by layer size read %d bytes", len(got))
	}
	if _, err := c.Blob("ai/smollm2", oci.Layer{Digest: "sha256:bad"}).OpenPrefix(t.Context(), 8); err == nil {
		t.Fatal("OpenPrefix() accepted an invalid digest")
	}
}

func TestParseReference(t *testing.T) {
	t.Parallel()
	good := map[string]oci.Reference{
		"ai/smollm2":         {Repository: "ai/smollm2", Tag: "latest"},
		"ai/smollm2:360M-Q4": {Repository: "ai/smollm2", Tag: "360M-Q4"},
		"ai/qwen3@sha256:" + strings.Repeat("a", 64): {Repository: "ai/qwen3", Digest: "sha256:" + strings.Repeat("a", 64)},
	}
	for in, want := range good {
		got, err := oci.ParseReference(in)
		if err != nil || got != want {
			t.Errorf("ParseReference(%q) = %+v, %v", in, got, err)
		}
		if err == nil && got.String() != strings.TrimSuffix(in, "") && want.Tag != "latest" {
			t.Errorf("String() = %q", got.String())
		}
	}
	for _, bad := range []string{"", "AI/Upper", "ai/../x", "ai//x", "ai/x:", "ai/x:-bad", "ai/x@sha256:short", "ai/x?y", "https://registry/ai/x", strings.Repeat("a", 300)} {
		if _, err := oci.ParseReference(bad); err == nil {
			t.Errorf("ParseReference(%q) accepted", bad)
		}
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	hc := fetchtest.Adapter(t, nil, "https://registry.test")
	authed := fetchtest.Adapter(t, httpclient.BearerAuth("x"), "https://registry.test")
	unpoliced, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]oci.Config{
		"no http":        {Registry: "https://registry.test"},
		"no policy":      {HTTP: unpoliced, Registry: "https://registry.test"},
		"bad registry":   {HTTP: hc, Registry: "ftp://registry.test"},
		"registry path":  {HTTP: hc, Registry: "https://registry.test/v2"},
		"adapter auth":   {HTTP: authed, Registry: "https://registry.test"},
		"negative bound": {HTTP: hc, Registry: "https://registry.test", MaxManifestBytes: -1},
	} {
		if _, err := oci.New(cfg); err == nil {
			t.Errorf("%s: New() accepted", name)
		}
	}
}

func TestResolveRefreshesRevokedTokenOnce(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	dockerModel(r)
	c := newClient(t, r)
	ref := oci.Reference{Repository: "ai/smollm2", Tag: "latest"}
	if _, err := c.Resolve(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.token = "tok-2"
	r.mu.Unlock()
	if _, err := c.Resolve(t.Context(), ref); err != nil {
		t.Fatalf("Resolve() after revocation = %v", err)
	}
	if r.tokens.Load() != 2 || r.challenge.Load() != 1 {
		t.Fatalf("challenges=%d tokens=%d", r.challenge.Load(), r.tokens.Load())
	}
}

func TestBlobRefreshesRevokedTokenOnce(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	d := r.addBlob([]byte("GGUF-weights"))
	c := newClient(t, r)
	read := func() error {
		rc, err := c.Blob("ai/smollm2", oci.Layer{Digest: d}).OpenPrefix(t.Context(), 4)
		if err != nil {
			return err
		}
		return rc.Close()
	}
	if err := read(); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.token = "tok-2"
	r.mu.Unlock()
	if err := read(); err != nil {
		t.Fatalf("OpenPrefix() after revocation = %v", err)
	}
	if r.tokens.Load() != 2 {
		t.Fatalf("tokens=%d, want 2", r.tokens.Load())
	}
}

func TestRepositoryChallengeFollowsAnonymousRoot(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.openRoot = true
	dockerModel(r)
	d := r.addBlob([]byte("GGUF-weights"))
	if _, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/smollm2", Tag: "latest"}); err != nil {
		t.Fatalf("Resolve() with an anonymous root = %v", err)
	}
	rc, err := newClient(t, r).Blob("ai/smollm2", oci.Layer{Digest: d}).OpenPrefix(t.Context(), 4)
	if err != nil {
		t.Fatalf("OpenPrefix() with an anonymous root = %v", err)
	}
	_ = rc.Close()
}

func TestTokenRealmOutsidePolicyIsNotContacted(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "stolen"})
	}))
	t.Cleanup(foreign.Close)
	r := newRegistry(t)
	r.realm = foreign.URL + "/token"
	dockerModel(r)
	if _, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/smollm2", Tag: "latest"}); err == nil {
		t.Fatal("Resolve() succeeded through an ungranted token realm")
	}
	if hits.Load() != 0 {
		t.Fatalf("ungranted realm contacted %d times", hits.Load())
	}
}

func TestResolveKeepsAnnotationsAndSeparatesProjector(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	cfg := []byte(`{"descriptor":{"name":"Gemma 3"},"config":{"format":"gguf"}}`)
	weight, proj := []byte("weights"), []byte("projector")
	r.addManifest("ai/gemma3", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config":        desc("application/vnd.cncf.model.config.v1+json", r.addBlob(cfg), len(cfg), nil),
		"layers": []any{
			desc("application/vnd.cncf.model.weight.v1.raw", r.addBlob(weight), len(weight), map[string]string{"org.cncf.model.filepath": "gemma-3-4b-Q4_K_M.gguf"}),
			desc("application/vnd.cncf.model.weight.v1.raw", r.addBlob(proj), len(proj), map[string]string{"org.cncf.model.filepath": "mmproj-F16.gguf"}),
		},
		"annotations": map[string]string{"ai.model.repo": "unsloth/gemma-3-4b-it-GGUF", "org.cncf.model.filepath": "gemma-3-4b-Q4_K_M.gguf"},
	})
	m, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/gemma3", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Annotations["ai.model.repo"] != "unsloth/gemma-3-4b-it-GGUF" {
		t.Fatalf("annotations = %v", m.Annotations)
	}
	// A vision projector is not part of the language weights, but a fit estimate for images must add it.
	if len(m.Weights) != 1 || m.WeightBytes != int64(len(weight)) || len(m.Files) != 1 || m.Files[0].Role != oci.RoleProjector {
		t.Fatalf("weights = %+v files = %+v", m.Weights, m.Files)
	}
}

func TestResolveRejectsOversizedAnnotations(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	cfg := []byte(`{"config":{"format":"gguf"}}`)
	r.addManifest("ai/x", "latest", "application/vnd.oci.image.manifest.v1+json", map[string]any{
		"schemaVersion": 2,
		"config":        desc("application/vnd.cncf.model.config.v1+json", r.addBlob(cfg), len(cfg), nil),
		"layers":        []any{},
		"annotations":   map[string]string{"k": strings.Repeat("v", 4097)},
	})
	_, err := newClient(t, r).Resolve(t.Context(), oci.Reference{Repository: "ai/x", Tag: "latest"})
	if app, ok := apperr.AsAppError(err); !ok || app.Reason != oci.ReasonMalformed {
		t.Fatalf("Resolve() error = %v, want malformed", err)
	}
}

func TestDigestUsesHeadNotManifestGet(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	dockerModel(r)
	c := newClient(t, r)
	got, err := c.Digest(t.Context(), oci.Reference{Repository: "ai/smollm2", Tag: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.Resolve(t.Context(), oci.Reference{Repository: "ai/smollm2", Digest: got})
	if err != nil || m.Digest != got {
		t.Fatalf("Digest() = %q, resolve = %v %v", got, m, err)
	}
	// Anonymous registries count manifest GETs against a pull quota; only the resolve above may GET.
	if r.gets.Load() != 1 {
		t.Fatalf("manifest GETs = %d, want 1", r.gets.Load())
	}
	r.mu.Lock()
	r.headerDig = "sha256:not-a-digest"
	r.mu.Unlock()
	if _, err := c.Digest(t.Context(), oci.Reference{Repository: "ai/smollm2", Tag: "latest"}); err == nil {
		t.Fatal("Digest() accepted an invalid stated digest")
	}
}
