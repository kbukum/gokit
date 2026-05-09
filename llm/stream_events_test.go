package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/llm"
)

func TestStreamErrorUnwrapAndEventInterfaces(t *testing.T) {
	err := llm.StreamError{Err: context.Canceled}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("unwrap should expose context.Canceled")
	}
	events := []llm.StreamEvent{llm.TextDelta{}, llm.ToolUseDelta{}, llm.ReasoningDelta{}, llm.UsageDelta{}, llm.MessageStart{}, llm.MessageComplete{}, llm.StreamError{}}
	if len(events) != 7 {
		t.Fatal(events)
	}
}
