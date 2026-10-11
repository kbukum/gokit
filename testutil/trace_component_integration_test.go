//go:build integration

package testutil_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/testutil"
)

func TestTraceComponentNativeOTLP(t *testing.T) {
	t.Parallel()
	for _, outage := range []bool{false, true} {
		name := "success"
		if outage {
			name = "outage"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			var active atomic.Int32
			closed := make(chan struct{}, 4)
			received := make(chan *collector.ExportTraceServiceRequest, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
				if err != nil || len(body) > 65536 {
					t.Errorf("invalid collector body: length %d, error %v", len(body), err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var request collector.ExportTraceServiceRequest
				if err := proto.Unmarshal(body, &request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.URL.Path != "/v1/traces" || r.Method != http.MethodPost {
					t.Error("unexpected OTLP route")
				}
				select {
				case received <- &request:
				default:
					t.Error("unexpected export retry within the bounded test window")
				}
				if outage {
					w.WriteHeader(http.StatusServiceUnavailable)
					if _, err := io.WriteString(w, "private-collector-sentinel"); err != nil {
						t.Error(err)
					}
					return
				}
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.WriteHeader(http.StatusOK)
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				switch state {
				case http.StateNew:
					active.Add(1)
				case http.StateClosed:
					active.Add(-1)
					select {
					case closed <- struct{}{}:
					default:
					}
				case http.StateActive, http.StateIdle, http.StateHijacked:
				}
			}
			server.Config.ReadHeaderTimeout = time.Second
			server.Config.ReadTimeout = time.Second
			server.Config.WriteTimeout = time.Second
			server.Start()
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
				defer cancel()
				if err := testutil.CloseHTTPServer(ctx, server); err != nil {
					t.Errorf("collector cleanup: %v", err)
				}
			})
			cfg := observability.DefaultTracerConfig("native-wire")
			cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
			cfg.ExportTimeout, cfg.FlushTimeout = 100*time.Millisecond, time.Second
			cfg.BatchSize = 1
			log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "collector", logging.WithWriter(&logs))
			if err != nil {
				t.Fatal(err)
			}
			c, err := observability.NewTraceComponent(cfg, log)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
				defer cancel()
				if err := c.Stop(ctx); err != nil {
					t.Errorf("tracing cleanup: %v", err)
				}
			})
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if err := c.Start(ctx); err != nil {
				t.Fatal(err)
			}
			_, span := c.Tracer("test").Start(ctx, "native.Call")
			span.End()
			select {
			case request := <-received:
				if len(request.ResourceSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans) != 1 ||
					len(request.ResourceSpans[0].ScopeSpans[0].Spans) != 1 ||
					request.ResourceSpans[0].ScopeSpans[0].Spans[0].Name != "native.Call" {
					t.Fatal("native collector did not receive the expected OTLP span")
				}
			case <-ctx.Done():
				t.Fatal("native collector not reached within budget")
			}
			start := time.Now()
			shutdown, stopShutdown := context.WithTimeout(ctx, cfg.FlushTimeout)
			defer stopShutdown()
			if err := c.Stop(shutdown); err != nil {
				t.Fatal(err)
			}
			if time.Since(start) > cfg.FlushTimeout {
				t.Fatal("native exporter exceeded shutdown budget")
			}
			for active.Load() != 0 {
				select {
				case <-closed:
				case <-shutdown.Done():
					t.Fatal("exporter left a live collector connection after shutdown")
				}
			}
			if strings.Contains(logs.String(), "private-") ||
				(outage && !strings.Contains(logs.String(), "TRACE_EXPORT_FAILED")) {
				t.Fatal("native export failure was silent or exposed private diagnostics")
			}
		})
	}
}
