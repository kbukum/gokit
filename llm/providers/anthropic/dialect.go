package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

func init() {
	llm.RegisterDialect("anthropic", &Dialect{})
}

// Dialect implements llm.Dialect for Anthropic's Messages API.
type Dialect struct{}

var _ llm.Dialect = (*Dialect)(nil)

func (d *Dialect) Name() string                    { return "anthropic" }
func (d *Dialect) ChatPath() string                { return "/v1/messages" }
func (d *Dialect) HealthPath() string              { return "/v1/messages" }
func (d *Dialect) StreamFormat() llm.StreamFormat   { return llm.StreamSSE }

// BuildRequest maps a universal CompletionRequest to the Anthropic JSON body.
func (d *Dialect) BuildRequest(req llm.CompletionRequest) (any, error) {
	messages := make([]map[string]any, 0, len(req.Messages))

	for _, m := range req.Messages {
		msg, err := encodeMessage(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}

	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	if req.Stream {
		body["stream"] = true
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	} else {
		body["max_tokens"] = 4096 // Anthropic requires max_tokens
	}

	if len(req.Tools) > 0 {
		body["tools"] = encodeTools(req.Tools)
	}
	if req.ToolChoice != nil {
		body["tool_choice"] = encodeToolChoice(req.ToolChoice)
	}
	if req.Extra != nil {
		for k, v := range req.Extra {
			body[k] = v
		}
	}

	return body, nil
}

// ParseResponse maps the Anthropic JSON response to a universal CompletionResponse.
func (d *Dialect) ParseResponse(body []byte) (*llm.CompletionResponse, error) {
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	msg := llm.AssistantMessage{}

	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			msg.Content = llm.TextContent(block.Text)
		case "tool_use":
			args := string(block.Input)
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	return &llm.CompletionResponse{
		Message: msg,
		Model:   raw.Model,
		Usage: llm.Usage{
			PromptTokens:     raw.Usage.InputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens:      raw.Usage.InputTokens + raw.Usage.OutputTokens,
		},
		StopReason: mapStopReason(raw.StopReason),
	}, nil
}

// ParseStreamChunk extracts content from an Anthropic SSE data payload.
func (d *Dialect) ParseStreamChunk(data []byte) (llm.StreamChunk, error) {
	var event struct {
		Type  string `json:"type"`
		Index int    `json:"index,omitempty"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json,omitempty"`
		} `json:"delta,omitempty"`
		ContentBlock struct {
			Type  string `json:"type"`
			ID    string `json:"id,omitempty"`
			Name  string `json:"name,omitempty"`
			Input any    `json:"input,omitempty"`
		} `json:"content_block,omitempty"`
	}

	if err := json.Unmarshal(data, &event); err != nil {
		return llm.StreamChunk{}, fmt.Errorf("anthropic: parse stream chunk: %w", err)
	}

	switch event.Type {
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			return llm.StreamChunk{
				ToolCalls: []llm.ToolCall{{
					ID:   event.ContentBlock.ID,
					Type: "function",
					Function: llm.FunctionCall{
						Name: event.ContentBlock.Name,
					},
				}},
			}, nil
		}
		return llm.StreamChunk{}, nil
	case "content_block_delta":
		if event.Delta.Type == "text_delta" {
			return llm.StreamChunk{Content: event.Delta.Text}, nil
		}
		if event.Delta.Type == "input_json_delta" {
			return llm.StreamChunk{
				ToolCalls: []llm.ToolCall{{
					Function: llm.FunctionCall{
						Arguments: event.Delta.PartialJSON,
					},
				}},
			}, nil
		}
		return llm.StreamChunk{}, nil
	case "message_stop":
		return llm.StreamChunk{Done: true}, nil
	default:
		return llm.StreamChunk{}, nil
	}
}

// --- internal helpers ---

func encodeMessage(m llm.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case llm.UserMessage:
		return map[string]any{
			"role":    "user",
			"content": llm.TextOf(msg.Content),
		}, nil
	case llm.AssistantMessage:
		if len(msg.ToolCalls) > 0 {
			blocks := make([]map[string]any, 0)
			text := llm.TextOf(msg.Content)
			if text != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
			for _, tc := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			return map[string]any{
				"role":    "assistant",
				"content": blocks,
			}, nil
		}
		return map[string]any{
			"role":    "assistant",
			"content": llm.TextOf(msg.Content),
		}, nil
	case llm.SystemMessage:
		return map[string]any{
			"role":    "user",
			"content": msg.Content,
		}, nil
	case llm.ToolResultMessage:
		return map[string]any{
			"role": "user",
			"content": []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": msg.ToolUseID,
					"content":     msg.Content,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("anthropic: unknown message type %T", m)
	}
}

func encodeTools(defs []tool.Definition) []map[string]any {
	var tools []map[string]any
	for _, d := range defs {
		tools = append(tools, map[string]any{
			"name":         d.Name,
			"description":  d.Description,
			"input_schema": d.InputSchema,
		})
	}
	return tools
}

func encodeToolChoice(tc *llm.ToolChoice) any {
	switch tc.Mode {
	case "auto":
		return map[string]any{"type": "auto"}
	case "none":
		return nil
	case "required":
		return map[string]any{"type": "any"}
	case "specific":
		return map[string]any{
			"type": "tool",
			"name": tc.Function,
		}
	default:
		return map[string]any{"type": "auto"}
	}
}

func mapStopReason(reason string) llm.StopReason {
	switch reason {
	case "end_turn":
		return llm.StopEndTurn
	case "tool_use":
		return llm.StopToolUse
	case "max_tokens":
		return llm.StopMaxTokens
	case "stop_sequence":
		return llm.StopEndTurn
	default:
		return llm.StopEndTurn
	}
}
