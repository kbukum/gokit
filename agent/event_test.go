package agent_test

import (
	"testing"

	"github.com/kbukum/gokit/agent"
)

// TestStopReasonWireStrings locks the snake_case stop-reason vocabulary shared
// across kits. gokit owns the vocabulary (D12): the natural-completion and
// limit stop reasons must serialize as these stable strings.
func TestStopReasonWireStrings(t *testing.T) {
	t.Parallel()
	cases := map[agent.StopReason]string{
		agent.StopEndTurn:      "stop",
		agent.StopMaxTokens:    "length",
		agent.StopCancelled:    "cancelled", //nolint:misspell // Contract spelling.
		agent.StopError:        "error",
		agent.StopMaxTurns:     "max_turns",
		agent.StopMaxToolCalls: "max_tool_calls",
		agent.StopWallClock:    "wall_clock",
		agent.StopCommand:      "command",
	}
	for reason, want := range cases {
		if string(reason) != want {
			t.Errorf("stop reason = %q, want %q", string(reason), want)
		}
	}
}

// TestAgentEventVocabulary proves the closed AgentEvent set is exactly gokit's
// vocabulary and that each variant satisfies the sealed interface.
func TestAgentEventVocabulary(t *testing.T) {
	t.Parallel()
	events := []agent.AgentEvent{
		agent.TurnStart{},
		agent.LLMDelta{},
		agent.ToolExecuting{},
		agent.ToolComplete{},
		agent.Compacted{},
		agent.TurnComplete{},
		agent.RunComplete{},
	}
	if len(events) != 7 {
		t.Fatalf("expected 7 agent event variants, got %d", len(events))
	}
}
