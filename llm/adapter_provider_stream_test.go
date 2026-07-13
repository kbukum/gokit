package llm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/llm/internal/streamwire"
)

func TestStreamEventsFromChunksPropagatesToolUseDecodeError(t *testing.T) {
	chunkCh := make(chan streamChunk, 1)
	chunkCh <- streamChunk{
		ToolCalls: []streamwire.ToolCall{{
			ID:         "call_1",
			Name:       "broken",
			InputDelta: `{"a":`,
		}},
		Done: true,
	}
	close(chunkCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := streamEventsFromChunks(ctx, chunkCh, "test-model", cancel)
	var (
		sawError    bool
		sawComplete bool
	)
	for event := range events {
		switch event.(type) {
		case StreamError:
			sawError = true
		case MessageComplete:
			sawComplete = true
		}
	}
	if !sawError {
		t.Fatal("expected StreamError")
	}
	if sawComplete {
		t.Fatal("did not expect MessageComplete after decode error")
	}
}

// TestStreamEventsFromChunksUnwindsOnContextCancel proves the emitter goroutine
// does not leak when a consumer abandons the event stream: once it cancels the
// context, every send unblocks, the emitter returns, closes the event channel,
// and invokes cancel to tear the producer down.
func TestStreamEventsFromChunksUnwindsOnContextCancel(t *testing.T) {
	// Queue more chunks than the event channel buffer (16) so the emitter
	// blocks on a send once the consumer stops reading.
	chunkCh := make(chan streamChunk, 64)
	for i := 0; i < 64; i++ {
		chunkCh <- streamChunk{Content: "x"}
	}
	close(chunkCh)

	ctx, cancel := context.WithCancel(context.Background())
	var canceled atomic.Bool
	events := streamEventsFromChunks(ctx, chunkCh, "test-model", func() {
		canceled.Store(true)
		cancel()
	})

	// Read a single event, then abandon the stream by canceling the context.
	<-events
	cancel()

	done := make(chan struct{})
	go func() {
		for range events { //nolint:revive // draining until closed
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("emitter did not unwind after context cancel (goroutine leak)")
	}
	if !canceled.Load() {
		t.Fatal("expected cancel to be invoked when the emitter unwinds")
	}
}
