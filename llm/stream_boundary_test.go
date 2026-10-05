package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/httpclient"
)

func TestStreamingNDJSONFrameBoundary(t *testing.T) {
	for _, limit := range []int{128, 1 << 20} {
		for _, extra := range []int{0, 1} {
			t.Run(fmt.Sprintf("%d/%d", limit, extra), func(t *testing.T) {
				prefix, suffix := `{"content":"`, `","done":true}`
				frame := prefix + strings.Repeat("x", limit-len(prefix)-len(suffix)+extra) + suffix
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/x-ndjson")
					fmt.Fprintln(w, frame)
				}))
				defer srv.Close()
				a, err := NewWithDialect(testDialect{}, Config{BaseURL: srv.URL, Stream: httpclient.StreamConfig{MaxFrameBytes: limit}})
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close(context.Background())
				ch, err := a.Stream(t.Context(), CompletionRequest{})
				if err != nil {
					t.Fatal(err)
				}
				complete := false
				var streamErr error
				for e := range ch {
					switch e := e.(type) {
					case MessageComplete:
						complete = true
					case StreamError:
						streamErr = e.Err
					}
				}
				if extra == 0 && (!complete || streamErr != nil) {
					t.Fatalf("boundary: %v complete=%v", streamErr, complete)
				}
				if extra == 1 && (complete || !errors.Is(streamErr, ErrStreamLimit)) {
					t.Fatalf("overflow: %v complete=%v", streamErr, complete)
				}
			})
		}
	}
}

func TestStreamAssemblyCumulativeLimits(t *testing.T) {
	a := newStreamAssembler("m", StreamLimits{MaxOutputBytes: 4, MaxToolArgsBytes: 4, MaxTools: 1})
	send := func(StreamEvent) error { return nil }
	for range 4 {
		if err := a.add(streamChunk{Reasoning: "x"}, send); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.add(streamChunk{Content: "x"}, send); !errors.Is(err, ErrStreamLimit) {
		t.Fatalf("cumulative limit: %v", err)
	}
	if a.content.Len() != 0 || a.reasoning.Len() != 4 {
		t.Fatal("assembly grew beyond limit")
	}
}
