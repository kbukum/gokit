package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

// Register installs the Anthropic dialect in the supplied registry.
// Call once at application startup before invoking [llm.New].
func Register(registry *llm.DialectRegistry) error {
	return registry.Register("anthropic", &Dialect{})
}

// Dialect implements llm.Dialect for Anthropic's Messages API.
type Dialect struct{}

var _ llm.Dialect = (*Dialect)(nil)

func (d *Dialect) Name() string                   { return "anthropic" }
func (d *Dialect) ChatPath() string               { return "/v1/messages" }
func (d *Dialect) HealthPath() string             { return "/v1/messages" }
func (d *Dialect) StreamFormat() llm.StreamFormat { return llm.StreamSSE }

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
		return nil, errors.New(errors.ErrCodeInvalidFormat, "anthropic: parse response", http.StatusBadGateway).WithCause(err)
	}

	msg := chat.AssistantMessage{}

	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			msg.Content = ai.TextContent(block.Text)
		case "tool_use":
			var input map[string]any
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &input)
			}
			if input == nil {
				input = map[string]any{}
			}
			msg.ToolCalls = append(msg.ToolCalls, ai.ToolUseBlock{
				ID:    block.ID,
				Name:  block.Name,
				Input: input,
			})
		}
	}

	return &llm.CompletionResponse{
		Message: msg,
		Model:   raw.Model,
		Usage: llm.Usage{
			InputTokens:  raw.Usage.InputTokens,
			OutputTokens: raw.Usage.OutputTokens,
		},
		StopReason: mapStopReason(raw.StopReason),
	}, nil
}

// ParseStreamChunk extracts content from an Anthropic SSE data payload.
func (d *Dialect) ParseStreamChunk(data []byte) (streamwire.Chunk, error) {
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
		return streamwire.Chunk{}, errors.New(errors.ErrCodeInvalidFormat, "anthropic: parse stream chunk", http.StatusBadGateway).WithCause(err)
	}

	switch event.Type {
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			return streamwire.Chunk{
				ToolCalls: []streamwire.ToolCall{{
					Index: event.Index,
					ID:    event.ContentBlock.ID,
					Name:  event.ContentBlock.Name,
				}},
			}, nil
		}
		return streamwire.Chunk{}, nil
	case "content_block_delta":
		if event.Delta.Type == "text_delta" {
			return streamwire.Chunk{Content: event.Delta.Text}, nil
		}
		if event.Delta.Type == "input_json_delta" {
			return streamwire.Chunk{
				ToolCalls: []streamwire.ToolCall{{
					Index:      event.Index,
					InputDelta: event.Delta.PartialJSON,
				}},
			}, nil
		}
		return streamwire.Chunk{}, nil
	case "message_stop":
		return streamwire.Chunk{Done: true}, nil
	default:
		return streamwire.Chunk{}, nil
	}
}

// --- internal helpers ---

func encodeMessage(m chat.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case chat.UserMessage:
		return map[string]any{
			"role":    "user",
			"content": ai.TextOf(msg.Content),
		}, nil
	case chat.AssistantMessage:
		if len(msg.ToolCalls) > 0 {
			blocks := make([]map[string]any, 0)
			text := ai.TextOf(msg.Content)
			if text != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
			for _, tb := range msg.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tb.ID,
					"name":  tb.Name,
					"input": tb.Input,
				})
			}
			return map[string]any{
				"role":    "assistant",
				"content": blocks,
			}, nil
		}
		return map[string]any{
			"role":    "assistant",
			"content": ai.TextOf(msg.Content),
		}, nil
	case chat.SystemMessage:
		return map[string]any{
			"role":    "user",
			"content": msg.Content,
		}, nil
	case chat.ToolResultMessage:
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
		return nil, errors.New(errors.ErrCodeInvalidInput, fmt.Sprintf("anthropic: unknown message type %T", m), http.StatusBadRequest)
	}
}

func encodeTools(defs []ai.ToolSpec) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
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

func mapStopReason(reason string) chat.FinishReason {
	switch reason {
	case "end_turn":
		return chat.FinishReasonStop
	case "tool_use":
		return chat.FinishReasonToolUse
	case "max_tokens":
		return chat.FinishReasonLength
	case "stop_sequence":
		return chat.FinishReasonStop
	default:
		return chat.FinishReasonStop
	}
}
