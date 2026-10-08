package testutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/sse"
)

type abortedAssertion struct{}

type assertionRecorder struct {
	testing.TB
	message string
}

func (r *assertionRecorder) Fatal(args ...any) {
	r.message = fmt.Sprint(args...)
	panic(abortedAssertion{})
}

func (r *assertionRecorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	panic(abortedAssertion{})
}

func expectFatal(t *testing.T, want string, call func(testing.TB)) {
	t.Helper()
	rec := &assertionRecorder{TB: t}
	defer func() {
		if _, ok := recover().(abortedAssertion); !ok {
			t.Error("helper did not abort with a test failure")
		}
		if !strings.Contains(rec.message, want) {
			t.Errorf("failure = %q, want %q", rec.message, want)
		}
	}()
	call(rec)
}

func TestHarnessReportsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	expectFatal(t, "limits", func(tb testing.TB) { New(tb, sse.Limits{}, sse.HandlerConfig{}) })
	expectFatal(t, "authorization", func(tb testing.TB) { New(tb, sse.DefaultLimits(), sse.HandlerConfig{}) })
}

func TestStreamAssertionsReportFailures(t *testing.T) {
	t.Parallel()
	expectFatal(t, "waiting for event", func(tb testing.TB) { parserFrom("").Require(tb, "item") })
	expectFatal(t, `expected event "item"`, func(tb testing.TB) { parserFrom("event: other\ndata: {}\n\n").Require(tb, "item") })
	expectFatal(t, "decoding event", func(tb testing.TB) {
		var result struct{}
		parserFrom("event: item\ndata: invalid\n\n").RequireJSON(tb, "item", &result)
	})
	stream := newStreamClient(&http.Response{StatusCode: http.StatusUnauthorized, Body: http.NoBody})
	expectFatal(t, "expected status", func(tb testing.TB) { RequireStatus(tb, stream, http.StatusOK) })
}

func TestHarnessReportsInvalidRequestAndConnectionFailure(t *testing.T) {
	t.Parallel()
	h := &Harness{Server: &httptest.Server{URL: "://invalid"}}
	if stream, err := h.Resume(t.Context(), "", ""); err == nil || stream != nil {
		t.Fatalf("invalid URL = %v, %v", stream, err)
	}
	expectFatal(t, "connecting SSE stream", func(tb testing.TB) { h.MustConnect(tb, t.Context(), "") })
}
