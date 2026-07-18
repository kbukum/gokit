package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/internal/streamwire"
	"github.com/kbukum/gokit/llm/providers/internal/dialect"
)

// Register installs the Gemini dialect in the supplied registry.
// Call once at application startup before invoking [llm.New].
func Register(registry *llm.DialectRegistry) error {
	return registry.Register("gemini", &Dialect{})
}

// Dialect implements llm.Dialect for Google's Gemini Generative AI API.
//
// Gemini uses a structurally different API from OpenAI/Anthropic:
//   - Content uses "parts" (text, functionCall, functionResponse)
//   - Endpoint path includes model name: /v1beta/models/{model}:generateContent
//   - Streaming uses SSE with :streamGenerateContent endpoint
//   - Tool definitions use different schema format
type Dialect struct{}

var _ llm.Dialect = (*Dialect)(nil)

func (d *Dialect) Name() string                   { return "gemini" }
func (d *Dialect) HealthPath() string             { return "" }
func (d *Dialect) StreamFormat() llm.StreamFormat { return llm.StreamSSE }

// ChatPath returns a placeholder. The actual path is model-dependent
// and set dynamically in BuildRequest via the model field.
// The adapter's base URL combined with this path forms the full endpoint.
//
// Gemini endpoint:
// /v1beta/models/{model}:generateContent The model name is injected by the adapter from CompletionRequest.Model.
func (d *Dialect) ChatPath() string {
	return "/v1beta/models"
}

// BuildRequest maps a universal CompletionRequest to the Gemini JSON body.
//
// Gemini API format:
//
//	{
//	 "contents": [{"role": "user", "parts": [{"text": "..."}]}],
//	 "systemInstruction": {"parts": [{"text": "..."}]},
//	 "tools": [{"functionDeclarations": [...]}],
//	 "generationConfig": {"temperature": 0.7, "maxOutputTokens": 1024}
//	}
func (d *Dialect) BuildRequest(req llm.CompletionRequest) (any, error) {
	contents := make([]map[string]any, 0, len(req.Messages))

	for _, m := range req.Messages {
		msg, err := encodeMessage(m)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			contents = append(contents, msg)
		}
	}

	body := map[string]any{
		"contents": contents,
	}

	if req.SystemPrompt != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": req.SystemPrompt},
			},
		}
	}

	genConfig := map[string]any{}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if len(req.StopSequences) > 0 {
		genConfig["stopSequences"] = req.StopSequences
	}
	if req.TopP != nil {
		genConfig["topP"] = *req.TopP
	}
	if len(genConfig) > 0 {
		body["generationConfig"] = genConfig
	}

	if len(req.Tools) > 0 {
		body["tools"] = []map[string]any{
			{"functionDeclarations": encodeTools(req.Tools)},
		}
	}

	// Embed the model in the body so the adapter can use it for the path
	body["_model"] = req.Model

	if err := dialect.MergeExtra(body, json.RawMessage(req.Extra)); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidInput, "gemini: invalid request extra", http.StatusBadRequest).WithCause(err)
	}

	return body, nil
}

// ParseResponse maps the Gemini JSON response to a universal CompletionResponse.
//
// Gemini response format:
//
//	{
//	 "candidates": [{"content": {"parts": [{"text": "..."}], "role": "model"}, "finishReason": "STOP"}],
//	 "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
//	}
func (d *Dialect) ParseResponse(body []byte) (*llm.CompletionResponse, error) {
	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text,omitempty"`
					FunctionCall *struct {
						Name string          `json:"name"`
						Args json.RawMessage `json:"args"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
				Role string `json:"role"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
		ModelVersion string `json:"modelVersion"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidFormat, "gemini: parse response", http.StatusBadGateway).WithCause(err)
	}

	if len(raw.Candidates) == 0 {
		return nil, errors.New(errors.ErrCodeInvalidFormat, "gemini: response has no candidates", http.StatusBadGateway)
	}

	candidate := raw.Candidates[0]
	msg := chat.AssistantMessage{}

	for i, part := range candidate.Content.Parts {
		if part.Text != "" {
			msg.Content = ai.TextContent(part.Text)
		}
		if part.FunctionCall != nil {
			msg.ToolCalls = append(msg.ToolCalls, ai.ToolUseBlock{
				ID:    fmt.Sprintf("call_%d", i),
				Name:  part.FunctionCall.Name,
				Input: ai.NormalizeToolInput(part.FunctionCall.Args),
			})
		}
	}

	model := raw.ModelVersion
	if model == "" {
		model = "gemini"
	}

	return &llm.CompletionResponse{
		Message: msg,
		Model:   model,
		Usage: llm.Usage{
			InputTokens:  raw.UsageMetadata.PromptTokenCount,
			OutputTokens: raw.UsageMetadata.CandidatesTokenCount,
		},
		StopReason: mapFinishReason(candidate.FinishReason),
	}, nil
}

// ParseStreamChunk extracts content from a Gemini SSE data payload.
//
// Gemini streaming returns the same structure as non-streaming but incrementally.
// Each chunk is a full candidates array with partial content.
func (d *Dialect) ParseStreamChunk(data []byte) (streamwire.Chunk, error) {
	var chunk struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text,omitempty"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args,omitempty"`
					} `json:"functionCall,omitempty"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason,omitempty"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &chunk); err != nil {
		return streamwire.Chunk{}, errors.New(errors.ErrCodeInvalidFormat, "gemini: parse stream chunk", http.StatusBadGateway).WithCause(err)
	}

	if len(chunk.Candidates) == 0 {
		return streamwire.Chunk{}, nil
	}

	candidate := chunk.Candidates[0]
	var text string
	var toolCalls []streamwire.ToolCall
	for i, part := range candidate.Content.Parts {
		text += part.Text
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, streamwire.ToolCall{
				Index:      i,
				ID:         fmt.Sprintf("call_%d", i),
				Name:       part.FunctionCall.Name,
				InputDelta: string(argsJSON),
			})
		}
	}

	done := candidate.FinishReason != "" && candidate.FinishReason != "NONE"
	return streamwire.Chunk{
		Content:   text,
		ToolCalls: toolCalls,
		Done:      done,
	}, nil
}

// --- internal helpers ---

func encodeMessage(m chat.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case chat.UserMessage:
		return map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"text": ai.TextOf(msg.Content)},
			},
		}, nil
	case chat.AssistantMessage:
		parts := make([]map[string]any, 0)
		text := ai.TextOf(msg.Content)
		if text != "" {
			parts = append(parts, map[string]any{"text": text})
		}
		for _, tb := range msg.ToolCalls {
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": tb.Name,
					"args": ai.NormalizeToolInput(tb.Input),
				},
			})
		}
		if len(parts) == 0 {
			parts = append(parts, map[string]any{"text": ""})
		}
		return map[string]any{
			"role":  "model",
			"parts": parts,
		}, nil
	case chat.SystemMessage:
		return nil, nil //nolint:nilnil // signals "no contents entry to emit"
	case chat.ToolResultMessage:
		return map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{
					"functionResponse": map[string]any{
						"name": msg.ToolUseID,
						"response": map[string]any{
							"content": msg.Content,
						},
					},
				},
			},
		}, nil
	default:
		return nil, errors.New(errors.ErrCodeInvalidInput, fmt.Sprintf("gemini: unknown message type %T", m), http.StatusBadRequest)
	}
}

func encodeTools(defs []ai.ToolSpec) []map[string]any {
	tools := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		t := map[string]any{
			"name":        d.Name,
			"description": d.Description,
		}
		if d.InputSchema != nil {
			t["parameters"] = d.InputSchema
		}
		tools = append(tools, t)
	}
	return tools
}

func mapFinishReason(reason string) chat.FinishReason {
	switch reason {
	case "STOP":
		return chat.FinishReasonStop
	case "MAX_TOKENS":
		return chat.FinishReasonLength
	case "SAFETY":
		return chat.FinishReasonContentFilter
	case "RECITATION":
		return chat.FinishReasonContentFilter
	case "TOOL_USE":
		return chat.FinishReasonToolUse
	default:
		return chat.FinishReasonStop
	}
}
