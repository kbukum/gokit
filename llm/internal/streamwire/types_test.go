package streamwire

import (
	"strings"
	"testing"
)

func TestToolUseBlocksRejectsInvalidJSON(t *testing.T) {
	_, err := ToolUseBlocks([]ToolCall{{
		ID:         "call_1",
		Name:       "broken",
		InputDelta: `{"a":`,
	}})
	if err == nil || !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("expected JSON error, got %v", err)
	}
}
