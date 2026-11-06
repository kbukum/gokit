package huggingface_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/huggingface"
	"github.com/kbukum/gokit/modelhub/internal/fetchtest"
	"github.com/kbukum/gokit/util"
)

var readAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

const searchBody = `[
 {"id":"bartowski/SmolLM2-360M-Instruct-GGUF","author":"bartowski","gated":false,"private":false,"sha":"7be6f65f1db715fe5dc5a4634c0d459b4eed42ec","likes":7,"downloads":23719,
  "tags":["gguf","license:apache-2.0","base_model:HuggingFaceTB/SmolLM2-360M-Instruct","base_model:quantized:HuggingFaceTB/SmolLM2-360M-Instruct"],
  "pipeline_tag":"text-generation","library_name":"gguf","createdAt":"2024-10-31T19:25:10.000Z","lastModified":"2024-10-31T19:38:27.000Z",
  "siblings":[{"rfilename":"README.md"},{"rfilename":"SmolLM2-360M-Instruct-Q4_K_M.gguf"}]},
 {"id":"meta-llama/Llama-3.2-1B","author":"meta-llama","gated":"manual","private":false,"sha":"4e20de362430cd3b72f300e6b0f18e50e7166e08","tags":["license:llama3.2"]},
 {"id":"x/y","gated":"something-new","sha":"zz"}
]`

const detailBody = `{"id":"HuggingFaceTB/SmolLM2-360M-Instruct-GGUF","author":"HuggingFaceTB","sha":"593b5a2e04c8f3e4ee880263f93e0bd2901ad47f","gated":"auto","private":false,
 "tags":["gguf","license:apache-2.0"],"library_name":"transformers","pipeline_tag":"text-generation",
 "cardData":{"license":["apache-2.0","mit"],"base_model":"HuggingFaceTB/SmolLM2-360M-Instruct"},
 "gguf":{"total":361821120,"architecture":"llama","context_length":8192,"chat_template":"{% for m in messages %}{% endfor %}"},
 "siblings":[
  {"rfilename":"README.md","blobId":"76fc3719e49b634855eb4881c9a31e6faa5210b9","size":1847},
  {"rfilename":"smollm2-360m-instruct-q8_0.gguf","blobId":"d6fa545f014b70945b12235c0cfbd4a9df694b22","size":386404992,"lfs":{"sha256":"48ab3034d0dd401fbc721eb1df3217902fee7dab9078992d66431f09b7750aaa","size":386404992}}
 ],
 "createdAt":"2024-10-31T20:37:31.000Z","lastModified":"2024-10-31T20:37:35.000Z"}`

type hub struct {
	srv      *httptest.Server
	requests atomic.Int32
	lastURL  atomic.Value
	handler  http.HandlerFunc
}

func newHub(t *testing.T, h http.HandlerFunc) *hub {
	t.Helper()
	s := &hub{handler: h}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.lastURL.Store(r.URL.String())
		s.handler(w, r)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func newClient(t *testing.T, endpoint string, auth *httpclient.AuthConfig) *huggingface.Client {
	t.Helper()
	hc := fetchtest.Adapter(t, auth, endpoint)
	c, err := huggingface.New(huggingface.Config{HTTP: hc, Endpoint: endpoint, Clock: util.NewFakeClock(readAt)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSearchReadsSummariesAndCursor(t *testing.T) {
	t.Parallel()
	var srvURL string
	h := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/models" || q.Get("search") != "smollm2" || q.Get("filter") != "gguf" || q.Get("limit") != "2" || q.Get("full") != "true" {
			http.Error(w, "bad query "+r.URL.String(), http.StatusBadRequest)
			return
		}
		if q.Get("cursor") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/models?search=smollm2&cursor=eyJhIjoxfQ%%3D%%3D>; rel="next"`, srvURL))
		}
		_, _ = io.WriteString(w, searchBody)
	})
	srvURL = h.srv.URL
	c := newClient(t, h.srv.URL, nil)
	page, err := c.Search(t.Context(), huggingface.Query{Text: "smollm2", Filter: "gguf", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Models) != 3 || page.Next != "eyJhIjoxfQ==" || !page.ReadAt.Equal(readAt) {
		t.Fatalf("page = %+v", page)
	}
	m := page.Models[0]
	if m.ID != "bartowski/SmolLM2-360M-Instruct-GGUF" || m.Gated != huggingface.GatedNone || m.SHA == "" || m.Downloads != 23719 || m.PipelineTag != "text-generation" {
		t.Fatalf("summary = %+v", m)
	}
	if len(m.Licenses) != 1 || m.Licenses[0] != "apache-2.0" || len(m.BaseModels) != 1 || m.BaseModels[0] != "HuggingFaceTB/SmolLM2-360M-Instruct" {
		t.Fatalf("licenses/base = %v %v", m.Licenses, m.BaseModels)
	}
	if len(m.Files) != 2 || m.Files[1].Path != "SmolLM2-360M-Instruct-Q4_K_M.gguf" || m.LastModified.IsZero() {
		t.Fatalf("files = %+v", m.Files)
	}
	if page.Models[1].Gated != huggingface.GatedManual || page.Models[2].Gated != huggingface.GatedUnknown {
		t.Fatalf("gated = %q %q", page.Models[1].Gated, page.Models[2].Gated)
	}
	next, err := c.Search(t.Context(), huggingface.Query{Text: "smollm2", Filter: "gguf", Limit: 2, Cursor: page.Next})
	if err != nil || next.Next != "" {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	if got := h.lastURL.Load().(string); !strings.Contains(got, "cursor=eyJhIjoxfQ%3D%3D") {
		t.Fatalf("cursor not sent: %s", got)
	}
}

func TestSearchSkipsMalformedItems(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"good/model"},{"id":"../bad"},{"id":"a/b","siblings":[{"rfilename":"../../etc/passwd"}]},"text"]`)
	})
	page, err := newClient(t, h.srv.URL, nil).Search(t.Context(), huggingface.Query{})
	if err != nil {
		t.Fatal(err)
	}
	// One hostile listing must not hide every other model on the page.
	if len(page.Models) != 1 || page.Models[0].ID != "good/model" || page.Skipped != 3 {
		t.Fatalf("page = %+v", page)
	}
}

func TestSearchIgnoresForeignNextLinks(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `<https://evil.test/api/models?cursor=abc>; rel="next"`)
		_, _ = io.WriteString(w, `[]`)
	})
	page, err := newClient(t, h.srv.URL, nil).Search(t.Context(), huggingface.Query{Text: "x"})
	if err != nil || page.Next != "" {
		t.Fatalf("Search() = %+v, %v", page, err)
	}
}

func TestSearchValidatesQuery(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) })
	c := newClient(t, h.srv.URL, nil)
	for name, q := range map[string]huggingface.Query{
		"limit":  {Limit: 101},
		"neg":    {Limit: -1},
		"cursor": {Cursor: "a b"},
		"text":   {Text: strings.Repeat("x", 300)},
		"sort":   {Sort: "evil"},
		"author": {Author: "a/b"},
	} {
		if _, err := c.Search(t.Context(), q); err == nil {
			t.Errorf("%s: Search() accepted", name)
		}
	}
	if h.requests.Load() != 0 {
		t.Fatalf("invalid queries reached the hub %d times", h.requests.Load())
	}
}

func TestModelReadsDetailsFileSizesAndGated(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/HuggingFaceTB/SmolLM2-360M-Instruct-GGUF/revision/main" || r.URL.Query().Get("blobs") != "true" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, detailBody)
	})
	m, err := newClient(t, h.srv.URL, nil).Model(t.Context(), "HuggingFaceTB/SmolLM2-360M-Instruct-GGUF", "main")
	if err != nil {
		t.Fatal(err)
	}
	if m.Gated != huggingface.GatedAuto || m.SHA != "593b5a2e04c8f3e4ee880263f93e0bd2901ad47f" || !m.ReadAt.Equal(readAt) {
		t.Fatalf("model = %+v", m)
	}
	if len(m.Licenses) != 2 || m.Licenses[1] != "mit" || len(m.BaseModels) != 1 {
		t.Fatalf("card = %v %v", m.Licenses, m.BaseModels)
	}
	if m.GGUF == nil || m.GGUF.Parameters != 361821120 || m.GGUF.Architecture != "llama" || m.GGUF.ContextLength != 8192 || !m.GGUF.HasChatTemplate {
		t.Fatalf("gguf = %+v", m.GGUF)
	}
	if len(m.Files) != 2 || m.Files[1].Size != 386404992 || m.Files[1].SHA256 == "" || m.Files[0].SHA256 != "" || m.Files[0].Size != 1847 {
		t.Fatalf("files = %+v", m.Files)
	}
}

func TestModelMapsMissingAndRestricted(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "missing"):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(r.URL.Path, "private"):
			w.WriteHeader(http.StatusUnauthorized)
		default:
			_, _ = io.WriteString(w, `{"id":"a/b","sha":"x","siblings":[{"rfilename":"../../etc/passwd","size":1}]}`)
		}
	})
	c := newClient(t, h.srv.URL, nil)
	for repo, code := range map[string]apperr.ErrorCode{"a/missing": apperr.ErrCodeNotFound, "a/private": apperr.ErrCodeNotFound} {
		if _, err := c.Model(t.Context(), repo, ""); err == nil {
			t.Errorf("%s: Model() succeeded", repo)
		} else if app, _ := apperr.AsAppError(err); app == nil || app.Code != code {
			t.Errorf("%s: error = %v", repo, err)
		}
	}
	if _, err := c.Model(t.Context(), "a/hostile", ""); err == nil {
		t.Fatal("Model() accepted a traversing file name")
	}
	for _, bad := range []string{"", "a", "a/b/c", "../x", "a/..", "a/b?c", "-a/b", strings.Repeat("a", 97) + "/b"} {
		if _, err := c.Model(t.Context(), bad, ""); err == nil {
			t.Errorf("Model(%q) accepted", bad)
		}
	}
	if _, err := c.Model(t.Context(), "a/b", "../main"); err == nil {
		t.Error("Model() accepted a traversing revision")
	}
}

func TestModelRejectsJunk(t *testing.T) {
	t.Parallel()
	h := newHub(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{not json`) })
	_, err := newClient(t, h.srv.URL, nil).Model(t.Context(), "a/b", "")
	if app, ok := apperr.AsAppError(err); !ok || app.Reason != huggingface.ReasonMalformed {
		t.Fatalf("junk error = %v", err)
	}
}

func TestFileReadsPrefixWithAdapterAuth(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("G", 64)
	h := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a/b/resolve/593b5a2e/dir/model%20q4.gguf" && r.URL.EscapedPath() != "/a/b/resolve/593b5a2e/dir/model%20q4.gguf" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer hf_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Range") != "bytes=0-15" {
			http.Error(w, "range "+r.Header.Get("Range"), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, payload)
	})
	c := newClient(t, h.srv.URL, httpclient.BearerAuth("hf_test"))
	src, err := c.File("a/b", "593b5a2e", "dir/model q4.gguf")
	if err != nil {
		t.Fatal(err)
	}
	rc, err := src.OpenPrefix(t.Context(), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	got, _ := io.ReadAll(rc)
	if len(got) != 16 {
		t.Fatalf("read %d bytes", len(got))
	}
	for _, bad := range []string{"", "../x", "/abs", `a\b`, "a/../b", "a//b", strings.Repeat("a", 1025)} {
		if _, err := c.File("a/b", "main", bad); err == nil {
			t.Errorf("File(%q) accepted", bad)
		}
	}
	unauth := newClient(t, h.srv.URL, nil)
	src, _ = unauth.File("a/b", "593b5a2e", "dir/model q4.gguf")
	if _, err := src.OpenPrefix(t.Context(), 16); err == nil {
		t.Fatal("OpenPrefix() succeeded without access")
	} else if app, _ := apperr.AsAppError(err); app == nil || app.Reason != huggingface.ReasonAccessRestricted {
		t.Fatalf("restricted error = %v", err)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	hc := fetchtest.Adapter(t, nil, "https://huggingface.co")
	unpoliced, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	withBase, err := httpclient.New(httpclient.Config{BaseURL: "https://huggingface.co"})
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]huggingface.Config{
		"no http":       {},
		"no policy":     {HTTP: unpoliced},
		"base url":      {HTTP: withBase},
		"bad endpoint":  {HTTP: hc, Endpoint: "ftp://hub.test"},
		"endpoint path": {HTTP: hc, Endpoint: "https://hub.test/api"},
		"negative":      {HTTP: hc, MaxResponseBytes: -1},
	} {
		if _, err := huggingface.New(cfg); err == nil {
			t.Errorf("%s: New() accepted", name)
		}
	}
	if _, err := huggingface.New(huggingface.Config{HTTP: hc}); err != nil {
		t.Fatalf("default endpoint: %v", err)
	}
}
