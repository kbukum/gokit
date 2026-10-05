package streamwire

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

type Chunk struct {
	Content    string             `json:"content"`
	Done       bool               `json:"done"`
	Err        error              `json:"-"`
	ToolCalls  []ToolCall         `json:"tool_calls,omitempty"`
	Reasoning  string             `json:"reasoning,omitempty"`
	Metadata   *chat.MessageStart `json:"metadata,omitempty"`
	Usage      *UsageUpdate       `json:"usage,omitempty"`
	StopReason chat.FinishReason  `json:"stop_reason,omitempty"`
}

// UsageUpdate carries cumulative provider accounting. Nil fields were not reported.
type UsageUpdate struct {
	InputTokens     *int
	OutputTokens    *int
	CachedTokens    *int
	ReasoningTokens *int
}

// Apply merges only reported counters; providers may report input and output separately.
func (u UsageUpdate) Apply(dst *ai.Usage) error {
	for _, pair := range []struct {
		src *int
		dst *int
	}{
		{u.InputTokens, &dst.InputTokens},
		{u.OutputTokens, &dst.OutputTokens},
		{u.CachedTokens, &dst.CachedTokens},
		{u.ReasoningTokens, &dst.ReasoningTokens},
	} {
		if pair.src != nil {
			if *pair.src < 0 {
				return fmt.Errorf("streamwire: negative token usage")
			}
			*pair.dst = *pair.src
		}
	}
	return nil
}

type ToolCall struct {
	Index      int    `json:"index"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	InputDelta string `json:"input_delta"`
}

func MergeToolDelta(calls []ToolCall, delta ToolCall) []ToolCall {
	if i := FindTool(calls, delta); i >= 0 {
		if delta.ID != "" {
			calls[i].ID = delta.ID
		}
		if delta.Name != "" {
			calls[i].Name = delta.Name
		}
		if delta.Index >= 0 {
			calls[i].Index = delta.Index
		}
		calls[i].InputDelta += delta.InputDelta
		return calls
	}
	return append(calls, delta)
}

// FindTool resolves a delta without allocating a new accumulated tool.
func FindTool(calls []ToolCall, delta ToolCall) int {
	for i := range calls {
		if delta.ID != "" && calls[i].ID == delta.ID {
			return i
		}
	}
	for i := range calls {
		if delta.Index >= 0 && calls[i].Index == delta.Index && (delta.ID == "" || calls[i].ID == "") {
			return i
		}
	}
	if delta.ID != "" || delta.Index >= 0 {
		return -1
	}
	if delta.Name != "" {
		for i := range calls {
			if calls[i].Name == delta.Name {
				return i
			}
		}
		return -1
	}
	return len(calls) - 1
}

// MaxToolArgsBytes bounds the total accumulated tool-call argument bytes for a single streamed message.
// Streamed tool arguments are untrusted model output;
// without a bound a server could stream unbounded deltas and exhaust memory.
// Stream assemblers abort the message with an error once the running total of [ToolArgsSize] exceeds this cap.
const MaxToolArgsBytes = 1 << 20 // 1 MiB

// ToolArgsSize returns the total accumulated InputDelta bytes across calls.
// It lets a stream assembler enforce [MaxToolArgsBytes] as deltas arrive.
func ToolArgsSize(calls []ToolCall) int {
	n := 0
	for i := range calls {
		n += len(calls[i].InputDelta)
	}
	return n
}

func ToolUseBlocks(calls []ToolCall) ([]ai.ToolUseBlock, error) {
	blocks := make([]ai.ToolUseBlock, 0, len(calls))
	for _, call := range calls {
		input := ai.NormalizeToolInput(json.RawMessage(call.InputDelta))
		if !json.Valid(input) {
			return nil, fmt.Errorf("streamwire: tool %q has invalid JSON arguments", call.Name)
		}
		blocks = append(blocks, ai.ToolUseBlock{
			ID:    call.ID,
			Name:  call.Name,
			Input: input,
		})
	}
	return blocks, nil
}
