package streamwire

import (
	"encoding/json"

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

func ToolUseBlocks(calls []ToolCall) ([]ai.ToolUseBlock, error) {
	blocks := make([]ai.ToolUseBlock, 0, len(calls))
	for _, call := range calls {
		var input map[string]any
		if call.InputDelta != "" {
			if err := json.Unmarshal([]byte(call.InputDelta), &input); err != nil {
				return nil, err
			}
		}
		if input == nil {
			input = map[string]any{}
		}
		blocks = append(blocks, ai.ToolUseBlock{ID: call.ID, Name: call.Name, Input: input})
	}
	return blocks, nil
}
