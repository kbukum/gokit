package lmstudio_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/workload/lmstudio"
)

// advanceUntil steps the fake clock until done closes, so clock-driven polls and backoff run without real sleeps.
func advanceUntil(clock *util.FakeClock, step time.Duration, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
			clock.Advance(step)
			time.Sleep(time.Millisecond)
		}
	}
}

func downloader(t *testing.T, srv *httptest.Server, clock util.TimerClock) *lmstudio.Downloader {
	t.Helper()
	d, err := lmstudio.NewDownloader(lmstudio.Config{BaseURL: srv.URL, Auth: httpclient.BearerAuth("lm-tok"), Clock: clock, PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStartPostsModelAndQuantization(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/models/download" || r.Header.Get("Authorization") != "Bearer lm-tok" ||
			body["model"] != "https://huggingface.co/lmstudio-community/gpt-oss-20b-GGUF" || body["quantization"] != "Q4_K_M" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"job_id":"job_493c7c9ded","status":"downloading","total_size_bytes":2279145003,"started_at":"2025-10-03T15:33:23.496Z"}`)
	}))
	t.Cleanup(srv.Close)
	job, err := downloader(t, srv, nil).Start(t.Context(), "https://huggingface.co/lmstudio-community/gpt-oss-20b-GGUF", "Q4_K_M")
	if err != nil {
		t.Fatal(err)
	}
	if job.JobID != "job_493c7c9ded" || job.Status != lmstudio.DownloadDownloading || job.TotalBytes != 2279145003 || job.StartedAt.IsZero() {
		t.Fatalf("job = %+v", job)
	}
	if calls.Load() != 1 {
		t.Fatalf("Start() sent %d requests", calls.Load())
	}
}

func TestStartReportsAlreadyDownloadedAndRefusesBadInput(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"status":"already_downloaded"}`)
	}))
	t.Cleanup(srv.Close)
	d := downloader(t, srv, nil)
	job, err := d.Start(t.Context(), "ibm/granite-4-micro", "")
	if err != nil || job.Status != lmstudio.DownloadAlreadyDownloaded || job.JobID != "" || !job.Status.Terminal() {
		t.Fatalf("Start() = %+v, %v", job, err)
	}
	for _, model := range []string{"", "a\nb", strings.Repeat("m", 2049)} {
		if _, err := d.Start(t.Context(), model, ""); err == nil {
			t.Errorf("Start(%q) accepted", model)
		}
	}
	if _, err := d.Start(t.Context(), "ibm/granite-4-micro", "Q4\x00"); err == nil {
		t.Error("Start() accepted a control character in the quantization")
	}
	for _, id := range []string{"", "../x", "a/b", "job?x", strings.Repeat("j", 200)} {
		if _, err := d.Status(t.Context(), id); err == nil {
			t.Errorf("Status(%q) accepted", id)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("invalid input reached the server: %d calls", calls.Load())
	}
}

func TestStartIsNotRetriedAndErrorsAreStatusOnly(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":"token lm-secret"}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	_, err := downloader(t, srv, nil).Start(t.Context(), "ibm/granite-4-micro", "")
	var apiErr *lmstudio.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || strings.Contains(err.Error(), "lm-secret") {
		t.Fatalf("Start() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("a download start was retried: %d calls", calls.Load())
	}
}

func TestFollowPollsOnClockUntilCompleted(t *testing.T) {
	t.Parallel()
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models/download/status/job_1" {
			http.NotFound(w, r)
			return
		}
		switch polls.Add(1) {
		case 1:
			// A transient failure is retried within the poll.
			w.WriteHeader(http.StatusBadGateway)
		case 2:
			_, _ = io.WriteString(w, `{"job_id":"job_1","status":"downloading","downloaded_bytes":10,"total_size_bytes":100,"bytes_per_second":5,"estimated_completion":"2025-10-03T15:43:12Z"}`)
		case 3:
			_, _ = io.WriteString(w, `{"job_id":"job_1","status":"paused","downloaded_bytes":50,"total_size_bytes":100}`)
		default:
			_, _ = io.WriteString(w, `{"job_id":"job_1","status":"completed","downloaded_bytes":100,"total_size_bytes":100,"completed_at":"2025-10-03T15:43:12.102Z"}`)
		}
	}))
	t.Cleanup(srv.Close)
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	d := downloader(t, srv, clock)
	var mu sync.Mutex
	var seen []lmstudio.DownloadStatus
	done := make(chan struct{})
	var final *lmstudio.DownloadJob
	var err error
	go func() {
		defer close(done)
		final, err = d.Follow(t.Context(), "job_1", func(j lmstudio.DownloadJob) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, j.Status)
		})
	}()
	advanceUntil(clock, time.Second, done)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != lmstudio.DownloadCompleted || final.DownloadedBytes != 100 || final.CompletedAt.IsZero() {
		t.Fatalf("final = %+v", final)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 || seen[0] != lmstudio.DownloadDownloading || seen[1] != lmstudio.DownloadPaused || seen[2] != lmstudio.DownloadCompleted {
		t.Fatalf("progress = %v", seen)
	}
}

func TestFollowReturnsTypedFailureAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/job_fail") {
			_, _ = io.WriteString(w, `{"job_id":"job_fail","status":"failed"}`)
			return
		}
		_, _ = io.WriteString(w, `{"job_id":"job_slow","status":"downloading","downloaded_bytes":1}`)
	}))
	t.Cleanup(srv.Close)
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	d := downloader(t, srv, clock)
	_, err := d.Follow(t.Context(), "job_fail", nil)
	var failed *lmstudio.DownloadFailedError
	if !errors.As(err, &failed) || failed.JobID != "job_fail" {
		t.Fatalf("Follow() failure = %v", err)
	}
	cctx, stop := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() {
		_, err := d.Follow(cctx, "job_slow", func(lmstudio.DownloadJob) { stop() })
		errc <- err
	}()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("Follow() after cancel = %v", err)
	}
}

func TestStatusRefusesMalformedJobs(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`not json`,
		`{"job_id":"job_1","status":"exploded"}`,
		`{"job_id":"job_1","status":"downloading","downloaded_bytes":-1}`,
		`{"job_id":"job_1","status":"downloading","bytes_per_second":-5}`,
		`{"job_id":"job_other","status":"downloading"}`,
		`{"job_id":"job/1","status":"downloading"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		_, err := downloader(t, srv, nil).Status(t.Context(), "job_1")
		srv.Close()
		if app, ok := apperr.AsAppError(err); !ok || app.Code != apperr.ErrCodeInvalidFormat {
			t.Errorf("%s: error = %v", body, err)
		}
	}
}

func TestDownloaderConfig(t *testing.T) {
	t.Parallel()
	hc, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]lmstudio.Config{
		"bad base":         {BaseURL: "ftp://x"},
		"short poll":       {BaseURL: "http://localhost:1234", PollInterval: time.Millisecond},
		"http and timeout": {BaseURL: "http://localhost:1234", HTTP: hc, Timeout: time.Second},
		"http and auth":    {BaseURL: "http://localhost:1234", HTTP: hc, Auth: httpclient.BearerAuth("x")},
	} {
		if _, err := lmstudio.NewDownloader(cfg); err == nil {
			t.Errorf("%s: NewDownloader() accepted", name)
		}
	}
	if _, err := lmstudio.NewDownloader(lmstudio.Config{BaseURL: "http://localhost:1234", HTTP: hc}); err != nil {
		t.Fatalf("injected adapter: %v", err)
	}
}

func TestStatusRetriesOnlyTransientFailures(t *testing.T) {
	t.Parallel()
	status := func(t *testing.T, handler http.HandlerFunc) (*lmstudio.DownloadJob, error) {
		t.Helper()
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		clock := util.NewFakeClock(time.Unix(0, 0))
		d := downloader(t, srv, clock)
		done := make(chan struct{})
		var job *lmstudio.DownloadJob
		var err error
		go func() {
			defer close(done)
			job, err = d.Status(context.Background(), "job_1")
		}()
		advanceUntil(clock, 500*time.Millisecond, done)
		return job, err
	}
	var flaky atomic.Int32
	job, err := status(t, func(w http.ResponseWriter, _ *http.Request) {
		if flaky.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// A job that omits its ID keeps the one asked for.
		_, _ = io.WriteString(w, `{"status":"paused","downloaded_bytes":5}`)
	})
	if err != nil || job.JobID != "job_1" || job.Status != lmstudio.DownloadPaused || flaky.Load() != 3 {
		t.Fatalf("Status() after transient failures = %+v, %v after %d calls", job, err, flaky.Load())
	}
	for code, wantCalls := range map[int]int32{http.StatusNotFound: 1, http.StatusInternalServerError: 3} {
		var calls atomic.Int32
		_, err := status(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(code)
		})
		apiErr, ok := errors.AsType[*lmstudio.Error](err)
		if !ok || apiErr.Status != code || calls.Load() != wantCalls {
			t.Errorf("status %d: Status() = %v after %d calls, want %d", code, err, calls.Load(), wantCalls)
		}
	}
}

func TestStatusReportsUnreachableServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	clock := util.NewFakeClock(time.Unix(0, 0))
	d := downloader(t, srv, clock)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = d.Status(context.Background(), "job_1")
	}()
	advanceUntil(clock, 500*time.Millisecond, done)
	if err == nil || !strings.Contains(err.Error(), "download status") {
		t.Fatalf("Status() of a closed server = %v", err)
	}
}
