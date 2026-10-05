package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

func TestStreamTotalIncludesAdmissionWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := util.NewFakeClock(time.Now())
		var requests, releases atomic.Int32
		c, err := New(Config{
			Stream: StreamConfig{TotalTimeout: time.Second},
			ResiliencePolicy: &resilience.Policy{Bulkhead: &resilience.BulkheadConfig{
				MaxConcurrent: 1, MaxWait: time.Hour, OnRelease: func(string) { releases.Add(1) },
			}},
		}, WithStreamClock(clock))
		if err != nil {
			t.Fatal(err)
		}
		c.httpClient.Transport = streamRoundTrip(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})
		req := Request{Method: "GET", Path: "http://example.test", RequireStreamCompletion: true}
		held, err := c.DoStream(t.Context(), req)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := c.DoStream(t.Context(), req); done <- err }()
		synctest.Wait()
		clock.Advance(time.Second)
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("admission timeout: %v", err)
		}
		if requests.Load() != 1 || releases.Load() != 0 {
			t.Fatal("waiter reached upstream or timeout released the protocol owner early")
		}
		if err := held.Complete(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if releases.Load() != 1 {
			t.Fatal("protocol completion retained capacity")
		}
	})
}
