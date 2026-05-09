package openai

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

// Register installs the OpenAI dialect in the supplied registry.
// Call once at application startup before invoking [llm.New].
func Register(registry *llm.DialectRegistry) error {
	return registry.Register("openai", &Dialect{})
}

// Dialect implements llm.Dialect for OpenAI-compatible APIs.
type Dialect struct{}

var _ llm.Dialect = (*Dialect)(nil)

func (d *Dialect) Name() string                   { return "openai" }
func (d *Dialect) ChatPath() string               { return "/v1/chat/completions" }
func (d *Dialect) HealthPath() string             { return "/v1/models" }
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
				Content          *string       `json:"content"`
				ReasoningContent *string       `json:"reasoning_content,omitempty"`
				ToolCalls        []rawToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokenCount     int `json:"prompt_tokens"`
			CompletionTokenCount int `json:"completion_tokens"`
			TotalTokens          int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidFormat, "openai: parse response", http.StatusBadGateway).WithCause(err)
	}

	if len(raw.Choices) == 0 {
		return nil, errors.New(errors.ErrCodeInvalidFormat, "openai: response has no choices", http.StatusBadGateway)
	}

	choice := raw.Choices[0]
	msg := chat.AssistantMessage{}

	if choice.Message.Content != nil && *choice.Message.Content != "" {
		msg.Content = ai.TextContent(*choice.Message.Content)
	} else if choice.Message.ReasoningContent != nil && *choice.Message.ReasoningContent != "" {
		// Some servers (DMR/llama.cpp with thinking models like qwen3, o1)
		// emit text under reasoning_content when content is empty.
		msg.Content = ai.TextContent(*choice.Message.ReasoningContent)
	}

	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		}
		if input == nil {
			input = map[string]any{}
		}
		msg.ToolCalls = append(msg.ToolCalls, ai.ToolUseBlock{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	return &llm.CompletionResponse{
		Message: msg,
		Model:   raw.Model,
		Usage: llm.Usage{
			InputTokens:  raw.Usage.PromptTokenCount,
			OutputTokens: raw.Usage.CompletionTokenCount,
		},
		StopReason: mapFinishReason(choice.FinishReason),
	}, nil
}

// ParseStreamChunk extracts content and tool calls from an SSE data payload.
func (d *Dialect) ParseStreamChunk(data []byte) (streamwire.Chunk, error) {
	s := string(data)
	if s == "[DONE]" {
		return streamwire.Chunk{Done: true}, nil
	}

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          string          `json:"content"`
				ReasoningContent string          `json:"reasoning_content,omitempty"`
				ToolCalls        []rawStreamTool `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(data, &chunk); err != nil {
		return streamwire.Chunk{}, errors.New(errors.ErrCodeInvalidFormat, "openai: parse stream chunk", http.StatusBadGateway).WithCause(err)
	}

	if len(chunk.Choices) == 0 {
		return streamwire.Chunk{}, nil
	}

	c := chunk.Choices[0]
	done := c.FinishReason != nil && *c.FinishReason != ""

	var toolCalls []streamwire.ToolCall
	for _, tc := range c.Delta.ToolCalls {
		toolCalls = append(toolCalls, streamwire.ToolCall{
			Index:      tc.Index,
			ID:         tc.ID,
			Name:       tc.Function.Name,
			InputDelta: tc.Function.Arguments,
		})
	}

	content := c.Delta.Content
	if content == "" && c.Delta.ReasoningContent != "" {
		content = c.Delta.ReasoningContent
	}

	return streamwire.Chunk{
		Content:   content,
		ToolCalls: toolCalls,
		Done:      done,
	}, nil
}

// rawStreamTool is the wire format for streaming tool call deltas.
type rawStreamTool struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
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

func encodeMessage(m chat.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case chat.UserMessage:
		return map[string]any{
			"role":    "user",
			"content": ai.TextOf(msg.Content),
		}, nil
	case chat.AssistantMessage:
		result := map[string]any{
			"role":    "assistant",
			"content": ai.TextOf(msg.Content),
		}
		if len(msg.ToolCalls) > 0 {
			var tcs []map[string]any
			for _, tb := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(tb.Input)
				tcs = append(tcs, map[string]any{
					"id":   tb.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tb.Name,
						"arguments": string(argsJSON),
					},
				})
			}
			result["tool_calls"] = tcs
		}
		return result, nil
	case chat.SystemMessage:
		return map[string]any{
			"role":    "system",
			"content": msg.Content,
		}, nil
	case chat.ToolResultMessage:
		return map[string]any{
			"role":         "tool",
			"content":      msg.Content,
			"tool_call_id": msg.ToolUseID,
		}, nil
	default:
		return nil, errors.New(errors.ErrCodeInvalidInput, fmt.Sprintf("openai: unknown message type %T", m), http.StatusBadRequest)
	}
}

func encodeTools(defs []ai.ToolSpec) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
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

func mapFinishReason(reason string) chat.FinishReason {
	switch reason {
	case "stop":
		return chat.FinishReasonStop
	case "tool_calls":
		return chat.FinishReasonToolUse
	case "length":
		return chat.FinishReasonLength
	case "content_filter":
		return chat.FinishReasonContentFilter
	default:
		return chat.FinishReasonStop
	}
}
