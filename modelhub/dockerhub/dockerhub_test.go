package dockerhub_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/modelhub/dockerhub"
	"github.com/kbukum/gokit/modelhub/internal/fetchtest"
	"github.com/kbukum/gokit/util"
)

const digest = "sha256:adb79096f87057a7dd58cdfb321b608c94e91778d8fc8b1bf3802de27278dc2e"

func newHub(t *testing.T, h http.HandlerFunc) (*dockerhub.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := dockerhub.New(dockerhub.Config{
		HTTP:     fetchtest.Adapter(t, nil, srv.URL),
		Endpoint: srv.URL,
		Clock:    util.NewFakeClock(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func reason(err error) string {
	app, ok := apperr.AsAppError(err)
	if !ok {
		return ""
	}
	return app.Reason
}

func TestRepositoriesReadsPageAndSkipsBadItems(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	c, srv := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v2/namespaces/ai/repositories" || q.Get("ordering") != "pull_count" || q.Get("page_size") != "2" || q.Get("name") != "gem" {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprintf(w, `{"count":118,"next":%q,"results":[
			{"name":"gemma3","namespace":"ai","description":"","star_count":68,"pull_count":1118825,"last_updated":"2026-08-05T11:25:32.724667Z","media_types":["application/vnd.cncf.model.manifest.v1+json"],"storage_size":277625461836},
			{"name":"../etc","namespace":"ai","pull_count":1},
			{"name":"other","namespace":"evil","pull_count":1},
			"not an object"]}`, srv.URL+"/v2/namespaces/ai/repositories?ordering=pull_count&page=2&page_size=2")
	})
	page, err := c.Repositories(t.Context(), "ai", dockerhub.Query{Text: "gem", Order: dockerhub.OrderPulls, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Skipped != 3 || page.Total != 118 || page.Next != 2 || page.ReadAt.IsZero() {
		t.Fatalf("page = %+v", page)
	}
	got := page.Items[0]
	if got.Name != "gemma3" || got.Pulls != 1118825 || got.Stars != 68 || got.StorageBytes != 277625461836 || got.LastUpdated.Year() != 2026 {
		t.Fatalf("repository = %+v", got)
	}
}

func TestNextLinkMustStayOnTheListing(t *testing.T) {
	t.Parallel()
	for name, next := range map[string]string{
		"other origin": "https://evil.example/v2/namespaces/ai/repositories?page=2",
		"other path":   "/v2/namespaces/other/repositories?page=2",
		"no page":      "/v2/namespaces/ai/repositories",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var srv *httptest.Server
			c, srv := newHub(t, func(w http.ResponseWriter, _ *http.Request) {
				link := next
				if strings.HasPrefix(next, "/") {
					link = srv.URL + next
				}
				fmt.Fprintf(w, `{"count":3,"next":%q,"results":[]}`, link)
			})
			if _, err := c.Repositories(t.Context(), "ai", dockerhub.Query{}); reason(err) != dockerhub.ReasonMalformed {
				t.Fatalf("Repositories() error = %v, want malformed next link", err)
			}
		})
	}
}

func TestTagsKeepDigestAndDownloadSize(t *testing.T) {
	t.Parallel()
	c, _ := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/namespaces/ai/repositories/gemma3/tags" || r.URL.Query().Get("page") != "3" {
			t.Errorf("request = %s", r.URL)
		}
		fmt.Fprintf(w, `{"count":34,"next":null,"results":[
			{"name":"latest","digest":%q,"media_type":"application/vnd.cncf.model.manifest.v1+json","full_size":8154978784,"tag_last_pushed":"2026-08-05T11:25:32.403653Z"},
			{"name":"bad","digest":"sha256:short","full_size":1},
			{"name":"neg","digest":%q,"full_size":-1}]}`, digest, digest)
	})
	page, err := c.Tags(t.Context(), "ai", "gemma3", dockerhub.Query{Page: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Skipped != 2 || page.Next != 0 {
		t.Fatalf("page = %+v", page)
	}
	if tag := page.Items[0]; tag.Name != "latest" || tag.Digest != digest || tag.Size != 8154978784 || tag.LastPushed.IsZero() {
		t.Fatalf("tag = %+v", tag)
	}
}

func TestRepositoryReadsOverviewWithinBound(t *testing.T) {
	t.Parallel()
	c, _ := newHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/namespaces/ai/repositories/big" {
			_, _ = io.WriteString(w, `{"name":"big","namespace":"ai","full_description":"`+strings.Repeat("a", 3<<20)+`"}`)
			return
		}
		_, _ = io.WriteString(w, `{"name":"gemma3","namespace":"ai","pull_count":5,"full_description":"<script>x</script> # Gemma"}`)
	})
	d, err := c.Repository(t.Context(), "ai", "gemma3")
	if err != nil {
		t.Fatal(err)
	}
	// The overview is returned as text; rendering it safely is the caller's job.
	if d.Name != "gemma3" || d.Pulls != 5 || d.Overview != "<script>x</script> # Gemma" {
		t.Fatalf("detail = %+v", d)
	}
	if _, err := c.Repository(t.Context(), "ai", "big"); reason(err) != "RESPONSE_TOO_LARGE" {
		t.Fatalf("Repository() error = %v, want the response bound", err)
	}
}

func TestRejectsInvalidInputBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	c, _ := newHub(t, func(_ http.ResponseWriter, r *http.Request) { t.Errorf("unexpected request %s", r.URL) })
	cases := []error{
		func() error { _, err := c.Repositories(t.Context(), "AI/../x", dockerhub.Query{}); return err }(),
		func() error { _, err := c.Repositories(t.Context(), "ai", dockerhub.Query{PageSize: 101}); return err }(),
		func() error { _, err := c.Repositories(t.Context(), "ai", dockerhub.Query{Order: "stars"}); return err }(),
		func() error { _, err := c.Tags(t.Context(), "ai", "x/y", dockerhub.Query{}); return err }(),
		func() error { _, err := c.Repository(t.Context(), "ai", ""); return err }(),
	}
	for i, err := range cases {
		if app, ok := apperr.AsAppError(err); !ok || app.Code != apperr.ErrCodeInvalidInput {
			t.Errorf("case %d error = %v, want invalid input", i, err)
		}
	}
}
