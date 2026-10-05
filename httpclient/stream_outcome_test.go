package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

func TestStreamProgressRenewsIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := util.NewFakeClock(time.Now())
		cfg := StreamConfig{FirstProgressTimeout: time.Second, IdleTimeout: 2 * time.Second}
		cfg.ApplyDefaults()
		life := newStreamLifetime(t.Context(), cfg, clock, true)
		life.setPhase("first_progress", cfg.FirstProgressTimeout)
		life.progress(false)
		clock.Advance(time.Second - time.Nanosecond)
		synctest.Wait()
		if life.ctx.Err() != nil {
			t.Fatal("early first-progress expiry")
		}
		life.progress(true)
		clock.Advance(time.Second)
		synctest.Wait()
		if life.ctx.Err() != nil {
			t.Fatal("model progress did not end first-progress timeout")
		}
		life.progress(false)
		clock.Advance(2*time.Second - time.Nanosecond)
		synctest.Wait()
		if life.ctx.Err() != nil {
			t.Fatal("chunk did not renew idle")
		}
		clock.Advance(time.Nanosecond)
		synctest.Wait()
		life.finish(context.Cause(life.ctx))
		<-life.watched
		if !errors.Is(life.outcome, context.DeadlineExceeded) {
			t.Fatalf("idle: %v", life.outcome)
		}
	})
}

func TestStreamBreakerTerminalOutcomes(t *testing.T) {
	for _, outcome := range []string{"success", "close", "cancel", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			cb := resilience.DefaultCircuitBreakerConfig("stream")
			cb.MaxFailures = 1
			p := &resilience.Policy{CircuitBreaker: &cb}
			c, err := New(Config{ResiliencePolicy: p})
			if err != nil {
				t.Fatal(err)
			}
			body := &countedBody{Reader: strings.NewReader("")}
			c.httpClient.Transport = streamRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			resp, err := c.DoStream(ctx, Request{Method: "GET", Path: "http://example.test", RequireStreamCompletion: true})
			if err != nil {
				t.Fatal(err)
			}
			if !c.IsAvailable(ctx) {
				t.Fatal("handshake affected breaker")
			}
			switch outcome {
			case "close":
				err = resp.Close()
			case "cancel":
				cancel()
				err = resp.Complete(context.Canceled)
			case "failure":
				err = resp.Complete(io.ErrUnexpectedEOF)
			default:
				err = resp.Complete(nil)
			}
			if outcome == "success" && err != nil {
				t.Fatal(err)
			}
			if body.closed.Load() != 1 {
				t.Fatalf("body closed %d times", body.closed.Load())
			}
			if c.IsAvailable(context.Background()) != (outcome != "failure") {
				t.Fatalf("breaker outcome %s: %v", outcome, err)
			}
		})
	}
}
