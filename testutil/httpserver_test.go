package testutil_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/testutil"
)

func TestFakeHTTPServer_ServesProgrammedResponse(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(
			http.StatusCreated,
			testutil.WithHeader("X-Trace", "abc123"),
			testutil.WithBodyString(`{"ok":true}`),
		),
	)
	defer srv.Close()

	if srv.URL() == "" {
		t.Fatal("URL() is empty")
	}
	if !strings.HasPrefix(srv.Addr(), "127.0.0.1:") {
		t.Fatalf("Addr() = %q, want a loopback host:port", srv.Addr())
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		srv.URL()+"/v1/widgets?verbose=1",
		strings.NewReader(`{"name":"gadget"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if got := resp.Header.Get("X-Trace"); got != "abc123" {
		t.Fatalf("X-Trace = %q, want abc123", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q, want {\"ok\":true}", body)
	}
}

func TestFakeHTTPServer_CapturesRequest(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(testutil.NewFakeResponse(http.StatusOK))
	defer srv.Close()

	if srv.CapturedRequest() != nil {
		t.Fatal("CapturedRequest() should be nil before any request")
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		srv.URL()+"/v1/things?page=2",
		strings.NewReader("payload-bytes"),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Api-Key", "k-42")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	captured := srv.CapturedRequest()
	if captured == nil {
		t.Fatal("CapturedRequest() is nil after a request")
	}
	if captured.Method != http.MethodPut {
		t.Fatalf("Method = %q, want PUT", captured.Method)
	}
	if captured.Path != "/v1/things" {
		t.Fatalf("Path = %q, want /v1/things", captured.Path)
	}
	if captured.RawQuery != "page=2" {
		t.Fatalf("RawQuery = %q, want page=2", captured.RawQuery)
	}
	if got := captured.HeaderValue("x-api-key"); got != "k-42" {
		t.Fatalf("HeaderValue(x-api-key) = %q, want k-42", got)
	}
	if string(captured.Body) != "payload-bytes" {
		t.Fatalf("Body = %q, want payload-bytes", captured.Body)
	}
}

func TestFakeHTTPServer_SecondRequestBeyondBudget(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(http.StatusOK, testutil.WithBodyString("first")),
	)
	defer srv.Close()

	first, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.StatusCode)
	}

	// A second request beyond the single-request budget is answered
	// deterministically (409 Conflict) rather than hanging or re-serving.
	second, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", second.StatusCode, http.StatusConflict)
	}
	if srv.RequestCount() != 2 {
		t.Fatalf("RequestCount() = %d, want 2", srv.RequestCount())
	}
	// The captured request remains the first one.
	if srv.CapturedRequest() == nil {
		t.Fatal("CapturedRequest() is nil")
	}
}

func TestFakeHTTPServer_ClosedIsDrainedAndRefusesConnections(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(testutil.NewFakeResponse(http.StatusOK))
	addr := srv.Addr()
	srv.Close()

	// After Close the listener is shut down: a new connection is refused,
	// proving no listener/goroutine is left running.
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", addr)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("dial to %s succeeded after Close, want connection refused", addr)
	}
}

func TestFakeHTTPServer_NilResponseDefaultsToOK(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a nil programmed response", resp.StatusCode)
	}
}

func TestFakeHTTPServer_OverBudgetRequestWithBodyRejectedAndCounted(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(testutil.NewFakeResponse(http.StatusOK))
	defer srv.Close()

	first, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Body.Close()

	// A second request carrying a body is still rejected deterministically with
	// 409 and is counted as received.
	second, err := http.Post(srv.URL(), "text/plain", strings.NewReader("over-budget-body"))
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second status = %d, want %d", second.StatusCode, http.StatusConflict)
	}
	if srv.RequestCount() != 2 {
		t.Fatalf("RequestCount() = %d, want 2", srv.RequestCount())
	}
	// The captured request remains the first (bodyless) GET.
	if got := srv.CapturedRequest(); got == nil || got.Method != http.MethodGet {
		t.Fatalf("CapturedRequest() = %+v, want the first GET", got)
	}
}

func TestFakeHTTPServer_ZeroMaxBodyBytesRejectsAnyBody(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(http.StatusOK),
		testutil.WithMaxBodyBytes(0),
	)
	defer srv.Close()

	resp, err := http.Post(srv.URL(), "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (zero limit must reject any body)", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestFakeHTTPServer_OversizedBodyRejectedWith413(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(http.StatusOK),
		testutil.WithMaxBodyBytes(8),
	)
	defer srv.Close()

	resp, err := http.Post(srv.URL(), "text/plain", strings.NewReader("this body is well over eight bytes"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestFakeHTTPServer_BodyWithinLimitCaptured(t *testing.T) {
	t.Parallel()

	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(http.StatusOK),
		testutil.WithMaxBodyBytes(64),
	)
	defer srv.Close()

	resp, err := http.Post(srv.URL(), "text/plain", strings.NewReader("small"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := srv.CapturedRequest(); got == nil {
		t.Fatal("captured request = nil, want request within body limit to be captured")
	} else if string(got.Body) != "small" {
		t.Fatalf("captured body = %q, want %q", got.Body, "small")
	}
}

func TestWithBody_SnapshotsCallerSlice(t *testing.T) {
	t.Parallel()

	buf := []byte("original")
	srv := testutil.NewFakeHTTPServer(
		testutil.NewFakeResponse(http.StatusOK, testutil.WithBody(buf)),
	)
	defer srv.Close()

	// Mutating the caller's buffer after construction must not change the
	// programmed response.
	for i := range buf {
		buf[i] = 'X'
	}

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "original" {
		t.Fatalf("body = %q, want %q", body, "original")
	}
}
