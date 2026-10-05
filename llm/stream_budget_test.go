package llm

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

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

type modelRoundTrip func(*http.Request) (*http.Response, error)

func (f modelRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStreamBudgetReleasesAbandonedConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := util.NewFakeClock(time.Now())
		var released atomic.Int32
		a, err := NewWithDialect(testDialect{}, Config{
			Stream:           httpclient.StreamConfig{FirstProgressTimeout: time.Second, IdleTimeout: time.Second},
			ResiliencePolicy: &resilience.Policy{Bulkhead: &resilience.BulkheadConfig{MaxConcurrent: 1, OnRelease: func(string) { released.Add(1) }}},
		})
		if err != nil {
			t.Fatal(err)
		}
		httpclient.WithStreamClock(clock)(a.REST().HTTP())
		closed := make(chan struct{})
		a.REST().HTTP().Unwrap().Transport = modelRoundTrip(func(r *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			go func() {
				_, writeErr := io.WriteString(writer, strings.Repeat("{\"content\":\"x\"}\n", 3))
				if writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) {
					t.Error(writeErr)
				}
				<-r.Context().Done()
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
				close(closed)
			}()
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, nil
		})
		events, err := a.Stream(t.Context(), CompletionRequest{})
		if err != nil {
			t.Fatal(err)
		}
		// Leave the one-slot output full; the reader must unwind on the idle budget anyway.
		synctest.Wait()
		clock.Advance(time.Second)
		synctest.Wait()
		if released.Load() != 1 {
			t.Fatal("timeout retained admission")
		}
		<-closed
		event, ok := <-events
		failure, isError := event.(StreamError)
		if !ok || !isError || !errors.Is(failure.Err, context.DeadlineExceeded) {
			t.Fatalf("terminal=%#v", event)
		}
		if _, ok := <-events; ok {
			t.Fatal("reader goroutine retained event channel")
		}
	})
}
