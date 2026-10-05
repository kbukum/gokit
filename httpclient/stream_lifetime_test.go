package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"

	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type streamRoundTrip func(*http.Request) (*http.Response, error)

func (f streamRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countedBody struct {
	io.Reader
	closed atomic.Int32
}

func (b *countedBody) Close() error { b.closed.Add(1); return nil }

func TestStreamLifetimeAdmissionAndOutcome(t *testing.T) {
	var released atomic.Int32
	policy := &resilience.Policy{Bulkhead: &resilience.BulkheadConfig{
		MaxConcurrent: 2,
		OnRelease:     func(string) { released.Add(1) },
	}}
	c, err := New(Config{ResiliencePolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	var bodies []*countedBody
	c.httpClient.Transport = streamRoundTrip(func(*http.Request) (*http.Response, error) {
		b := &countedBody{Reader: strings.NewReader("ok")}
		bodies = append(bodies, b)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: b}, nil
	})
	req := Request{Method: "GET", Path: "http://example.test", RequireStreamCompletion: true}
	one, err := c.DoStream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	two, err := c.DoStream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.DoStream(t.Context(), req); !errors.Is(err, resilience.ErrBulkheadFull) {
		t.Fatalf("third stream: %v", err)
	}
	if _, err = io.ReadAll(one.Body); err != nil {
		t.Fatal(err)
	}
	if released.Load() != 0 {
		t.Fatal("EOF certified protocol success")
	}
	if err = one.Complete(nil); err != nil {
		t.Fatal(err)
	}
	if err = one.Complete(errors.New("late failure")); err != nil {
		t.Fatal(err)
	}
	if released.Load() != 1 || bodies[0].closed.Load() != 1 {
		t.Fatal("completion not exactly once")
	}
	if err = two.Close(); err != nil {
		t.Fatal(err)
	}
	if released.Load() != 2 || bodies[1].closed.Load() != 1 {
		t.Fatal("close leaked admission/body")
	}
}

func TestStreamLifetimeBudgets(t *testing.T) {
	for _, phase := range []string{"connect", "headers", "first_progress", "idle", "total"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := util.NewFakeClock(time.Now())
				cfg := StreamConfig{HeaderTimeout: time.Hour, FirstProgressTimeout: time.Hour, IdleTimeout: time.Hour, TotalTimeout: time.Hour}
				switch phase {
				case "connect":
					cfg.ConnectTimeout = time.Second
				case "headers":
					cfg.HeaderTimeout = time.Second
				case "first_progress":
					cfg.FirstProgressTimeout = time.Second
				case "idle":
					cfg.IdleTimeout = time.Second
				case "total":
					cfg.TotalTimeout = time.Second
				}
				c, err := New(Config{Stream: cfg}, WithStreamClock(clock))
				if err != nil {
					t.Fatal(err)
				}
				closed := make(chan struct{})
				c.httpClient.Transport = streamRoundTrip(func(req *http.Request) (*http.Response, error) {
					if phase == "connect" || phase == "headers" {
						if phase == "connect" {
							httptrace.ContextClientTrace(req.Context()).GetConn("example.test")
						}
						<-req.Context().Done()
						close(closed)
						return nil, req.Context().Err()
					}

					r, w := io.Pipe()
					go func() {
						<-req.Context().Done()
						if err := w.CloseWithError(req.Context().Err()); err != nil {
							t.Error(err)
						}
						close(closed)
					}()
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: r}, nil
				})
				done := make(chan error, 1)
				go func() {
					resp, err := c.DoStream(t.Context(), Request{Method: "GET", Path: "http://example.test", RequireStreamCompletion: true})
					if err == nil {
						if phase == "idle" {
							resp.Progress(true)
						}
						_, err = io.ReadAll(resp.Body)
						err = resp.Complete(err)
					}
					done <- err
				}()
				synctest.Wait()
				clock.Advance(time.Second - time.Nanosecond)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("early expiry: %v", err)
				default:
				}
				clock.Advance(time.Nanosecond)
				synctest.Wait()
				select {
				case err := <-done:
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("expiry: %v", err)
					}
				default:
					t.Fatal("budget did not expire")
				}
				<-closed
			})
		})
	}
}
