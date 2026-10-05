package sse_test

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/util"
)

// A quiet HTTP/2 stream must survive past its write budget: the budget bounds each write, not the idle wait between
// frames.
func TestEndpointQuietHTTP2StreamOutlivesWriteTimeout(t *testing.T) {
	t.Parallel()
	b := endpointBus(t)
	cfg := handlerConfig()
	cfg.Clock = util.SystemClock{}
	cfg.WriteTimeout = 100 * time.Millisecond
	cfg.Heartbeat = 600 * time.Millisecond
	h, err := sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	client := srv.Client()
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 || resp.StatusCode != http.StatusOK {
		t.Fatalf("response = %s %d", resp.Proto, resp.StatusCode)
	}
	lines := bufio.NewScanner(resp.Body)
	for lines.Scan() {
		if strings.HasPrefix(lines.Text(), ": keepalive") {
			return
		}
	}
	t.Fatalf("stream ended before heartbeat: %v", lines.Err())
}
