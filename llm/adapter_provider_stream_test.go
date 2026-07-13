package llm

import (
	"context"
	"testing"

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

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := streamEventsFromChunks(chunkCh, "test-model", cancel)
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
