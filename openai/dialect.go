package openai

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

func init() {
	llm.RegisterDialect("openai", &Dialect{})
}

// Dialect implements llm.Dialect for OpenAI-compatible APIs.
type Dialect struct{}

var _ llm.Dialect = (*Dialect)(nil)

func (d *Dialect) Name() string       { return "openai" }
func (d *Dialect) ChatPath() string   { return "/v1/chat/completions" }
func (d *Dialect) HealthPath() string { return "/v1/models" }
func (d *Dialect) StreamFormat() llm.StreamFormat { return llm.StreamSSE }

// BuildRequest maps a universal CompletionRequest to the OpenAI JSON body.
func (d *Dialect) BuildRequest(req llm.CompletionRequest) (any, error) {
	messages := make([]map[string]any, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.SystemPrompt,
		})
	}

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
		"stream":   req.Stream,
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
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

// ParseResponse maps the OpenAI JSON response to a universal CompletionResponse.
func (d *Dialect) ParseResponse(body []byte) (*llm.CompletionResponse, error) {
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   *string        `json:"content"`
				ToolCalls []rawToolCall  `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}

	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("openai: response has no choices")
	}

	choice := raw.Choices[0]
	msg := llm.AssistantMessage{}

	if choice.Message.Content != nil && *choice.Message.Content != "" {
		msg.Content = llm.TextContent(*choice.Message.Content)
	}

	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: llm.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return &llm.CompletionResponse{
		Message: msg,
		Model:   raw.Model,
		Usage: llm.Usage{
			PromptTokens:     raw.Usage.PromptTokens,
			CompletionTokens: raw.Usage.CompletionTokens,
			TotalTokens:      raw.Usage.TotalTokens,
		},
		StopReason: mapFinishReason(choice.FinishReason),
	}, nil
}

// ParseStreamChunk extracts content from an SSE data payload.
func (d *Dialect) ParseStreamChunk(data []byte) (string, bool, error) {
	s := string(data)
	if s == "[DONE]" {
		return "", true, nil
	}

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(data, &chunk); err != nil {
		return "", false, fmt.Errorf("openai: parse stream chunk: %w", err)
	}

	if len(chunk.Choices) == 0 {
		return "", false, nil
	}

	c := chunk.Choices[0]
	done := c.FinishReason != nil && *c.FinishReason != ""
	return c.Delta.Content, done, nil
}

// --- internal helpers ---

type rawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func encodeMessage(m llm.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case llm.UserMessage:
		return map[string]any{
			"role":    "user",
			"content": llm.TextOf(msg.Content),
		}, nil
	case llm.AssistantMessage:
		result := map[string]any{
			"role":    "assistant",
			"content": llm.TextOf(msg.Content),
		}
		if len(msg.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tc := range msg.ToolCalls {
				tcs = append(tcs, map[string]any{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]any{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				})
			}
			result["tool_calls"] = tcs
		}
		return result, nil
	case llm.SystemMessage:
		return map[string]any{
			"role":    "system",
			"content": msg.Content,
		}, nil
	case llm.ToolResultMessage:
		return map[string]any{
			"role":         "tool",
			"content":      msg.Content,
			"tool_call_id": msg.ToolUseID,
		}, nil
	default:
		return nil, fmt.Errorf("openai: unknown message type %T", m)
	}
}

func encodeTools(defs []tool.Definition) []map[string]any {
	var tools []map[string]any
	for _, d := range defs {
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  d.InputSchema,
			},
		})
	}
	return tools
}

func encodeToolChoice(tc *llm.ToolChoice) any {
	switch tc.Mode {
	case "auto":
		return "auto"
	case "none":
		return "none"
	case "required":
		return "required"
	case "specific":
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": tc.Function},
		}
	default:
		return "auto"
	}
}

func mapFinishReason(reason string) llm.StopReason {
	switch reason {
	case "stop":
		return llm.StopEndTurn
	case "tool_calls":
		return llm.StopToolUse
	case "length":
		return llm.StopMaxTokens
	case "content_filter":
		return llm.StopContentFilter
	default:
		return llm.StopEndTurn
	}
}
