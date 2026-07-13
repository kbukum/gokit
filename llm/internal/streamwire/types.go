package streamwire

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
)

type Chunk struct {
	Content   string     `json:"content"`
	Done      bool       `json:"done"`
	Err       error      `json:"-"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Index      int    `json:"index"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	InputDelta string `json:"input_delta"`
}

func MergeToolDelta(calls []ToolCall, delta ToolCall) []ToolCall {
	if delta.ID != "" {
		for i := range calls {
			if calls[i].ID == delta.ID {
				if delta.Name != "" {
					calls[i].Name = delta.Name
				}
				if delta.Index >= 0 {
					calls[i].Index = delta.Index
				}
				calls[i].InputDelta += delta.InputDelta
				return calls
			}
		}
		return append(calls, delta)
	}
	if delta.Index >= 0 {
		for i := range calls {
			if calls[i].Index == delta.Index {
				if delta.Name != "" {
					calls[i].Name = delta.Name
				}
				calls[i].InputDelta += delta.InputDelta
				return calls
			}
		}
		return append(calls, delta)
	}
	if delta.Name != "" {
		for i := range calls {
			if calls[i].Name == delta.Name {
				calls[i].InputDelta += delta.InputDelta
				return calls
			}
		}
		return append(calls, delta)
	}
	if len(calls) > 0 {
		calls[len(calls)-1].InputDelta += delta.InputDelta
		return calls
	}
	return append(calls, delta)
}

// MaxToolArgsBytes bounds the total accumulated tool-call argument bytes for a
// single streamed message. Streamed tool arguments are untrusted model output;
// without a bound a server could stream unbounded deltas and exhaust memory.
// Stream assemblers abort the message with an error once the running total of
// [ToolArgsSize] exceeds this cap.
const MaxToolArgsBytes = 1 << 20 // 1 MiB

// ToolArgsSize returns the total accumulated InputDelta bytes across calls. It
// lets a stream assembler enforce [MaxToolArgsBytes] as deltas arrive.
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
