package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/kbukum/gokit/httpclient"
)

func TestStreamAssemblerPropagatesToolUseDecodeError(t *testing.T) {
	a := newStreamAssembler("test", defaultStreamLimits())
	err := a.add(streamChunk{ToolCalls: []streamToolCall{{ID: "1", Name: "broken", InputDelta: `{"a":`}}, Done: true}, func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.complete(); err == nil {
		t.Fatal("expected tool decode error")
	}
}

func TestStreamDeliveryUnwindsOnContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		a, err := NewWithDialect(testDialect{}, Config{})
		if err != nil {
			t.Fatal(err)
		}
		body := &testStreamBody{Reader: strings.NewReader(strings.Repeat("{\"content\":\"x\"}\n", 64))}
		resp := &httpclient.StreamResponse{Body: body}
		out := make(chan StreamEvent, 1)
		// Consume through the public HTTP response context below; an unmanaged response has no cancellation seam, so exercise the send callback directly.
		done := make(chan error, 1)
		go func() {
			_, err := a.consumeStream(resp, "m", func(e StreamEvent) error {
				select {
				case out <- e:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			done <- err
		}()
		<-out
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v", err)
		}
	})
}

type testStreamBody struct{ *strings.Reader }

func (*testStreamBody) Close() error { return nil }
