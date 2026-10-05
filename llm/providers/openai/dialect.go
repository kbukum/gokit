package openai

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/internal/streamwire"
	"github.com/kbukum/gokit/llm/providers/internal/dialect"
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
	if req.Stream {
		body["stream_options"] = map[string]bool{"include_usage": true}
	}

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.StopSequences) > 0 {
		body["stop"] = req.StopSequences
	}
	if len(req.Tools) > 0 {
		body["tools"] = encodeTools(req.Tools)
	}
	if req.ToolChoice != nil {
		body["tool_choice"] = encodeToolChoice(req.ToolChoice)
	}
	if err := dialect.MergeExtra(body, json.RawMessage(req.Extra)); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidInput, "openai: invalid request extra").WithCause(err)
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
		Usage *struct {
			PromptTokenCount     int `json:"prompt_tokens"`
			CompletionTokenCount int `json:"completion_tokens"`
			TotalTokens          int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New(errors.ErrCodeExternalService, "openai: parse response").WithCause(err)
	}

	if len(raw.Choices) == 0 {
		return nil, errors.New(errors.ErrCodeExternalService, "openai: response has no choices")
	}

	choice := raw.Choices[0]
	msg := chat.AssistantMessage{}

	if choice.Message.Content != nil && *choice.Message.Content != "" {
		msg.Content = ai.TextContent(*choice.Message.Content)
	}

	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ai.ToolUseBlock{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: ai.NormalizeToolInput(json.RawMessage(tc.Function.Arguments)),
		})
	}

	result := &llm.CompletionResponse{
		Message:    msg,
		Model:      raw.Model,
		ID:         raw.ID,
		StopReason: mapFinishReason(choice.FinishReason),
	}
	if choice.Message.ReasoningContent != nil {
		result.Reasoning = *choice.Message.ReasoningContent
	}
	if raw.Usage != nil {
		result.UsageReported = true
		result.Usage = llm.Usage{
			InputTokens:  raw.Usage.PromptTokenCount,
			OutputTokens: raw.Usage.CompletionTokenCount,
		}
	}
	return result, nil
}

// ParseStreamChunk extracts content and tool calls from an SSE data payload.
func (d *Dialect) ParseStreamChunk(data []byte) (streamwire.Chunk, error) {
	s := string(data)
	if s == "[DONE]" {
		return streamwire.Chunk{Done: true}, nil
	}

	var chunk struct {
		ID    string          `json:"id"`
		Model string          `json:"model"`
		Error json.RawMessage `json:"error"`
		Usage *struct {
			Input         *int `json:"prompt_tokens"`
			Output        *int `json:"completion_tokens"`
			PromptDetails struct {
				Cached *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionDetails struct {
				Reasoning *int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
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
		return streamwire.Chunk{}, errors.New(errors.ErrCodeExternalService, "openai: parse stream chunk").WithCause(err)
	}
	if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
		return streamwire.Chunk{}, errors.New(errors.ErrCodeExternalService, "openai: upstream stream failure")
	}
	result := streamwire.Chunk{}
	if chunk.ID != "" || chunk.Model != "" {
		result.Metadata = &chat.MessageStart{ID: chunk.ID, Model: chunk.Model}
	}
	if chunk.Usage != nil {
		result.Usage = &streamwire.UsageUpdate{InputTokens: chunk.Usage.Input, OutputTokens: chunk.Usage.Output, CachedTokens: chunk.Usage.PromptDetails.Cached, ReasoningTokens: chunk.Usage.CompletionDetails.Reasoning}
	}
	if len(chunk.Choices) == 0 {
		return result, nil
	}

	c := chunk.Choices[0]
	if c.FinishReason != nil && *c.FinishReason != "" {
		result.StopReason = mapFinishReason(*c.FinishReason)
	}

	var toolCalls []streamwire.ToolCall
	for _, tc := range c.Delta.ToolCalls {
		toolCalls = append(toolCalls, streamwire.ToolCall{
			Index:      tc.Index,
			ID:         tc.ID,
			Name:       tc.Function.Name,
			InputDelta: tc.Function.Arguments,
		})
	}

	result.Content = c.Delta.Content
	result.Reasoning = c.Delta.ReasoningContent
	result.ToolCalls = toolCalls
	return result, nil
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
				tcs = append(tcs, map[string]any{
					"id":   tb.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tb.Name,
						"arguments": string(ai.NormalizeToolInput(tb.Input)),
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
		return nil, errors.New(errors.ErrCodeInvalidInput, fmt.Sprintf("openai: unknown message type %T", m))
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
