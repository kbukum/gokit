package fetch_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/internal/fetch"
	"github.com/kbukum/gokit/modelhub/internal/fetchtest"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

func newClient(t *testing.T, clock util.TimerClock, origin string) *fetch.Client {
	t.Helper()
	hc := fetchtest.Adapter(t, nil, origin)
	c, err := fetch.New("test source", fetch.Config{HTTP: hc, Clock: clock, Retry: fetch.DefaultRetry()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// advanceUntil advances the fake clock until done closes, so retry backoffs fire deterministically.
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

func appCode(t *testing.T, err error) *apperr.AppError {
	t.Helper()
	app, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("error %v is not an AppError", err)
	}
	return app
}

func TestGetReadsBoundedBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "x" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("request lost query or header: %s %v", r.URL, r.Header)
		}
		w.Header().Set("X-Total", "7")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	c := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL)
	resp, err := c.Get(t.Context(), fetch.Request{URL: srv.URL, Query: map[string]string{"q": "x"}, Headers: map[string]string{"Accept": "application/json"}, MaxBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != `{"ok":true}` || resp.Header("x-total") != "7" || resp.Status != http.StatusOK {
		t.Fatalf("Get() = %+v", resp)
	}
}

func TestGetRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("a", 65))
	}))
	defer srv.Close()
	_, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Get(t.Context(), fetch.Request{URL: srv.URL, MaxBytes: 64})
	if app := appCode(t, err); app.Reason != fetch.ReasonResponseTooLarge || app.Retryable {
		t.Fatalf("Get() error = %+v, want non-retryable too-large", app)
	}
}

func TestGetMapsStatusWithoutBody(t *testing.T) {
	t.Parallel()
	cases := map[int]apperr.ErrorCode{
		http.StatusNotFound:     apperr.ErrCodeNotFound,
		http.StatusUnauthorized: apperr.ErrCodeUnauthorized,
		http.StatusForbidden:    apperr.ErrCodeForbidden,
		http.StatusBadRequest:   apperr.ErrCodeExternalService,
	}
	for status, want := range cases {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "secret-body")
		}))
		resp, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Get(t.Context(), fetch.Request{URL: srv.URL, MaxBytes: 64})
		srv.Close()
		app := appCode(t, err)
		if app.Code != want || calls.Load() != 1 || fetch.StatusOf(err) != status {
			t.Errorf("status %d: code=%s calls=%d status=%d", status, app.Code, calls.Load(), fetch.StatusOf(err))
		}
		if strings.Contains(err.Error(), "secret-body") || resp == nil || resp.Status != status {
			t.Errorf("status %d: error leaked body or lost response: %v", status, err)
		}
	}
}

func TestGetRetriesTransientFailuresOnInjectedClock(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	clock := util.NewFakeClock(time.Unix(0, 0))
	c := newClient(t, clock, srv.URL)
	done := make(chan struct{})
	var body []byte
	var err error
	go func() {
		defer close(done)
		var resp *fetch.Response
		resp, err = c.Get(context.Background(), fetch.Request{URL: srv.URL, MaxBytes: 8})
		if resp != nil {
			body = resp.Body
		}
	}()
	advanceUntil(clock, time.Second, done)
	if err != nil || string(body) != "ok" || calls.Load() != 3 {
		t.Fatalf("Get() = %q, %v after %d calls", body, err, calls.Load())
	}
}

func TestGetHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for name, header := range map[string]func() string{
		"seconds": func() string { return "4" },
		"date":    func() string { return clock.Now().Add(4 * time.Second).Format(http.TimeFormat) },
	} {
		var calls atomic.Int32
		var gap atomic.Int64
		first := clock.Now()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				first = clock.Now()
				w.Header().Set("Retry-After", header())
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			gap.Store(int64(clock.Now().Sub(first)))
		}))
		c := newClient(t, clock, srv.URL)
		done := make(chan struct{})
		var err error
		go func() {
			defer close(done)
			_, err = c.Get(context.Background(), fetch.Request{URL: srv.URL, MaxBytes: 8})
		}()
		advanceUntil(clock, time.Second, done)
		srv.Close()
		if err != nil || time.Duration(gap.Load()) < 4*time.Second {
			t.Errorf("%s: Get() = %v, retried after %v, want >= 4s", name, err, time.Duration(gap.Load()))
		}
	}
}

func TestOpenHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	clock := util.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	var calls atomic.Int32
	var gap atomic.Int64
	first := clock.Now()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			first = clock.Now()
			w.Header().Set("Retry-After", "4")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		gap.Store(int64(clock.Now().Sub(first)))
		_, _ = io.WriteString(w, "GGUF")
	}))
	defer srv.Close()
	c := newClient(t, clock, srv.URL)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		var body io.ReadCloser
		if body, _, err = c.Open(context.Background(), fetch.Request{URL: srv.URL}, 4); err == nil {
			_ = body.Close()
		}
	}()
	advanceUntil(clock, time.Second, done)
	if err != nil || time.Duration(gap.Load()) < 4*time.Second {
		t.Fatalf("Open() = %v, retried after %v, want >= 4s", err, time.Duration(gap.Load()))
	}
}

func TestOpenSendsRangeAndBoundsBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-3" {
			t.Errorf("Range = %q", r.Header.Get("Range"))
		}
		// A server that ignores the range still cannot push more than the prefix.
		_, _ = io.WriteString(w, "0123456789")
	}))
	defer srv.Close()
	body, _, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Open(t.Context(), fetch.Request{URL: srv.URL}, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "0123" {
		t.Fatalf("Open() read %q, %v", got, err)
	}
}

func TestOpenMapsStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Trace", "t1")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, resp, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Open(t.Context(), fetch.Request{URL: srv.URL}, 4)
	if app := appCode(t, err); app.Code != apperr.ErrCodeNotFound || resp == nil || resp.Status != http.StatusNotFound || resp.Header("X-Trace") != "t1" {
		t.Fatalf("Open() = %+v, %v", resp, err)
	}
	if _, _, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Open(t.Context(), fetch.Request{URL: srv.URL}, 0); err == nil {
		t.Fatal("Open() accepted an empty prefix")
	}
}

func TestNewRejectsUnsafeAdapters(t *testing.T) {
	t.Parallel()
	retrying, err := httpclient.New(httpclient.Config{ResiliencePolicy: resilience.NewPolicy().WithRetry(resilience.DefaultRetryConfig())})
	if err != nil {
		t.Fatal(err)
	}
	based, err := httpclient.New(httpclient.Config{BaseURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	granted := fetchtest.Adapter(t, nil, "http://127.0.0.1:1")
	for name, cfg := range map[string]fetch.Config{
		"nil adapter":     {},
		"adapter retries": {HTTP: retrying},
		"adapter base":    {HTTP: based},
		"no policy":       {HTTP: plain},
		"bad retry":       {HTTP: granted, Retry: resilience.RetryConfig{MaxAttempts: -1}},
	} {
		if _, err := fetch.New("test", cfg); err == nil {
			t.Errorf("%s: New() accepted an unsafe config", name)
		}
	}
}

func TestStatusOfNonHTTPError(t *testing.T) {
	t.Parallel()
	if fetch.StatusOf(errors.New("plain")) != 0 {
		t.Fatal("StatusOf() invented a status")
	}
}

func TestGetClassifiesTransportFailures(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	granted := srv.URL
	srv.Close()
	clock := util.NewFakeClock(time.Unix(0, 0))
	c := newClient(t, clock, granted)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = c.Get(context.Background(), fetch.Request{URL: granted, MaxBytes: 8})
	}()
	advanceUntil(clock, time.Second, done)
	if app := appCode(t, err); app.Code != apperr.ErrCodeConnectionFailed {
		t.Fatalf("closed server error = %v", err)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer other.Close()
	if _, err = c.Get(t.Context(), fetch.Request{URL: other.URL, MaxBytes: 8}); err == nil {
		t.Fatal("Get() reached an origin outside the policy")
	} else if _, ok := apperr.AsAppError(err); !ok {
		t.Fatalf("policy refusal is untyped: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = c.Get(ctx, fetch.Request{URL: granted, MaxBytes: 8}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Get() = %v", err)
	}
}

func TestGetMarksExhaustedServerErrors(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	clock := util.NewFakeClock(time.Unix(0, 0))
	c := newClient(t, clock, srv.URL)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = c.Get(context.Background(), fetch.Request{URL: srv.URL, MaxBytes: 8})
	}()
	advanceUntil(clock, time.Second, done)
	app := appCode(t, err)
	if !app.Retryable || app.Details["retries_exhausted"] != true || fetch.StatusOf(err) != http.StatusInternalServerError || calls.Load() != 3 {
		t.Fatalf("Get() = %v (details %v) after %d calls", err, app.Details, calls.Load())
	}
}

func TestGetStopsReadingAtTheBound(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("a", 65))
		w.(http.Flusher).Flush()
		// A hostile source never ends its body; the client must stop at the bound instead of waiting for the end.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := newClient(t, util.NewFakeClock(time.Unix(0, 0)), srv.URL).Get(ctx, fetch.Request{URL: srv.URL, MaxBytes: 64})
	if app := appCode(t, err); app.Reason != fetch.ReasonResponseTooLarge {
		t.Fatalf("Get() error = %v, want the size bound before the source ends its body", err)
	}
}

func TestHeadReadsHeadersWithoutBody(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:abc")
	}))
	defer srv.Close()
	clock := util.NewFakeClock(time.Unix(0, 0))
	c := newClient(t, clock, srv.URL)
	done := make(chan struct{})
	var resp *fetch.Response
	var err error
	go func() {
		defer close(done)
		resp, err = c.Head(t.Context(), fetch.Request{URL: srv.URL})
	}()
	advanceUntil(clock, time.Second, done)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header("docker-content-digest") != "sha256:abc" || calls.Load() != 2 {
		t.Fatalf("Head() = %+v after %d calls", resp, calls.Load())
	}
}
