package providers_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/openai"
	"github.com/kbukum/gokit/resilience"
)

// TestStreamingIntegration drives the real client and provider over owned loopback sockets.
func TestStreamingIntegration(t *testing.T) {
	for _, scenario := range []string{"success", "headers", "first-progress", "idle", "cancel", "unterminated", "malformed", "provider-error", "incomplete", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			var requests, active, released atomic.Int32
			upstreamDone := make(chan struct{})
			stop := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				active.Add(1)
				defer active.Add(-1)
				defer close(upstreamDone)
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Error(err)
					return
				}
				wait := func() {
					select {
					case <-r.Context().Done():
					case <-stop:
					}
				}
				if scenario == "headers" {
					wait()
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				flusher := w.(http.Flusher)
				flusher.Flush()
				write := func(s string) bool {
					if _, err := io.WriteString(w, s); err != nil {
						return false
					}
					flusher.Flush()
					return true
				}
				switch scenario {
				case "success":
					write("data: {\"id\":\"response\",\"model\":\"actual\",\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n")
					write("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"length\"}]}\n\n")
					write("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":2}}\n\n")
					write("data: [DONE]\n\n")
				case "first-progress":
					write("data: {\"id\":\"metadata\",\"choices\":[]}\n\n")
					wait()
				case "idle", "cancel":
					write("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n")
					wait()
				case "unterminated":
					write("data: {")
					wait()
				case "malformed":
					write("data: {\n\n")
				case "provider-error":
					write("data: {\"error\":{\"message\":\"synthetic failure\"}}\n\n")
				case "incomplete":
					write("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
				case "overflow":
					for range 5 {
						if !write("data: {\"choices\":[{\"delta\":{\"content\":\"xx\"}}]}\n\n") {
							return
						}
					}
					wait()
				}
			}))
			t.Cleanup(func() { close(stop); srv.Close() })
			policy := &resilience.Policy{
				CircuitBreaker: &resilience.CircuitBreakerConfig{MaxFailures: 1},
				Bulkhead: &resilience.BulkheadConfig{MaxConcurrent: 1, OnRelease: func(string) {
					released.Add(1)
				}},
				Retry: &resilience.RetryConfig{MaxAttempts: 3},
			}
			cfg := llm.Config{
				BaseURL: srv.URL, Model: "requested", ResiliencePolicy: policy,
				Stream: httpclient.StreamConfig{HeaderTimeout: 100 * time.Millisecond, FirstProgressTimeout: 100 * time.Millisecond, IdleTimeout: 100 * time.Millisecond, TotalTimeout: time.Second},
			}
			if scenario == "overflow" {
				cfg.StreamLimits.MaxOutputBytes = 8
			}
			adapter, err := llm.NewWithDialect(&openai.Dialect{}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := adapter.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			events, callErr := llm.NewProvider(adapter, "requested").Stream(ctx, llm.CompletionRequest{})
			var result *llm.CompletionResponse
			var content, reasoning string
			usageEvents := 0
			if callErr == nil {
				for e := range events {
					switch e := e.(type) {
					case llm.TextDelta:
						content += e.Text
					case llm.ReasoningDelta:
						reasoning += e.Text
						if scenario == "cancel" {
							cancel()
						}
					case llm.UsageDelta:
						usageEvents++
					case llm.StreamError:
						callErr = e.Err
					case llm.MessageComplete:
						result = &e.Response
					}
				}
			}
			select {
			case <-upstreamDone:
			case <-ctx.Done():
				// Cancellation is intentional in one scenario; allow a fresh bounded teardown wait.
				select {
				case <-upstreamDone:
				case <-time.After(2 * time.Second):
					t.Fatal("upstream not released")
				}
			}
			if requests.Load() != 1 || active.Load() != 0 || released.Load() != 1 {
				t.Fatalf("resources: requests=%d active=%d releases=%d", requests.Load(), active.Load(), released.Load())
			}
			wantHealthy := scenario == "success" || scenario == "cancel"
			if adapter.REST().HTTP().GetConfig().ResiliencePolicy.IsAvailable() != wantHealthy {
				t.Fatalf("breaker does not reflect terminal %s outcome", scenario)
			}
			if scenario == "success" {
				if callErr != nil || result == nil || result.Text() != "ok" || content != "ok" || reasoning != "think" || result.Reasoning != "think" || !result.UsageReported || result.Usage.OutputTokens != 2 || result.Model != "actual" || result.StopReason != "length" || usageEvents != 1 {
					t.Fatalf("result=%+v content=%q reasoning=%q usageEvents=%d err=%v", result, content, reasoning, usageEvents, callErr)
				}
			} else if callErr == nil || result != nil {
				t.Fatalf("failure looked successful: result=%+v err=%v", result, callErr)
			}
			if scenario == "cancel" && !errors.Is(callErr, context.Canceled) {
				t.Fatalf("cancel: %v", callErr)
			}
			if scenario == "overflow" && !errors.Is(callErr, llm.ErrStreamLimit) {
				t.Fatalf("overflow: %v", callErr)
			}
			if scenario == "incomplete" && !errors.Is(callErr, io.ErrUnexpectedEOF) {
				t.Fatalf("EOF: %v", callErr)
			}
			if strings.Contains(scenario, "progress") || scenario == "idle" || scenario == "headers" || scenario == "unterminated" {
				if !errors.Is(callErr, context.DeadlineExceeded) {
					t.Fatalf("timeout: %v", callErr)
				}
			}
		})
	}
}

func TestStreamingSSEFrameBoundary(t *testing.T) {
	const limit = 1 << 20
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			prefix := `data: {"choices":[{"delta":{"content":"`
			suffix := `"}}]}`
			frame := prefix + strings.Repeat("x", limit-len(prefix)-len(suffix)-2+extra) + suffix + "\n\n"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, frame, "data: [DONE]\n\n")
			}))
			defer srv.Close()
			a, err := llm.NewWithDialect(&openai.Dialect{}, llm.Config{BaseURL: srv.URL, Stream: httpclient.StreamConfig{MaxFrameBytes: limit}})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close(context.Background())
			events, err := a.Stream(t.Context(), llm.CompletionRequest{})
			if err != nil {
				t.Fatal(err)
			}
			complete, failed := false, false
			for e := range events {
				switch e.(type) {
				case llm.MessageComplete:
					complete = true
				case llm.StreamError:
					failed = true
				}
			}
			if complete != (extra == 0) || failed != (extra == 1) {
				t.Fatalf("boundary complete=%v failed=%v", complete, failed)
			}
		})
	}
}
