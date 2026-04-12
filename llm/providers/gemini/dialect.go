package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

func init() {
	llm.RegisterDialect("gemini", &Dialect{})
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

// ChatPath returns a placeholder. The actual path is model-dependent and set
// dynamically in BuildRequest via the model field. The adapter's base URL
// combined with this path forms the full endpoint.
//
// Gemini endpoint: /v1beta/models/{model}:generateContent
// The model name is injected by the adapter from CompletionRequest.Model.
func (d *Dialect) ChatPath() string {
	return "/v1beta/models"
}

// BuildRequest maps a universal CompletionRequest to the Gemini JSON body.
//
// Gemini API format:
//
//	{
//	  "contents": [{"role": "user", "parts": [{"text": "..."}]}],
//	  "systemInstruction": {"parts": [{"text": "..."}]},
//	  "tools": [{"functionDeclarations": [...]}],
//	  "generationConfig": {"temperature": 0.7, "maxOutputTokens": 1024}
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

	if req.Extra != nil {
		for k, v := range req.Extra {
			body[k] = v
		}
	}

	return body, nil
}

// ParseResponse maps the Gemini JSON response to a universal CompletionResponse.
//
// Gemini response format:
//
//	{
//	  "candidates": [{"content": {"parts": [{"text": "..."}], "role": "model"}, "finishReason": "STOP"}],
//	  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
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
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}

	if len(raw.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: response has no candidates")
	}

	candidate := raw.Candidates[0]
	msg := llm.AssistantMessage{}

	for i, part := range candidate.Content.Parts {
		if part.Text != "" {
			msg.Content = llm.TextContent(part.Text)
		}
		if part.FunctionCall != nil {
			args := string(part.FunctionCall.Args)
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
				ID:   fmt.Sprintf("call_%d", i),
				Type: "function",
				Function: llm.FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: args,
				},
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
			PromptTokens:     raw.UsageMetadata.PromptTokenCount,
			CompletionTokens: raw.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      raw.UsageMetadata.TotalTokenCount,
		},
		StopReason: mapFinishReason(candidate.FinishReason),
	}, nil
}

// ParseStreamChunk extracts content from a Gemini SSE data payload.
//
// Gemini streaming returns the same structure as non-streaming but incrementally.
// Each chunk is a full candidates array with partial content.
func (d *Dialect) ParseStreamChunk(data []byte) (llm.StreamChunk, error) {
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
		return llm.StreamChunk{}, fmt.Errorf("gemini: parse stream chunk: %w", err)
	}

	if len(chunk.Candidates) == 0 {
		return llm.StreamChunk{}, nil
	}

	candidate := chunk.Candidates[0]
	var text string
	var toolCalls []llm.ToolCall
	for _, part := range candidate.Content.Parts {
		text += part.Text
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, llm.ToolCall{
				Type: "function",
				Function: llm.FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}

	done := candidate.FinishReason != "" && candidate.FinishReason != "NONE"
	return llm.StreamChunk{
		Content:   text,
		ToolCalls: toolCalls,
		Done:      done,
	}, nil
}

// --- internal helpers ---

func encodeMessage(m llm.Message) (map[string]any, error) {
	switch msg := m.(type) {
	case llm.UserMessage:
		return map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"text": llm.TextOf(msg.Content)},
			},
		}, nil
	case llm.AssistantMessage:
		parts := make([]map[string]any, 0)
		text := llm.TextOf(msg.Content)
		if text != "" {
			parts = append(parts, map[string]any{"text": text})
		}
		for _, tc := range msg.ToolCalls {
			var args any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{}
			}
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": tc.Function.Name,
					"args": args,
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
	case llm.SystemMessage:
		// System messages are handled via systemInstruction, skip in contents
		return nil, nil
	case llm.ToolResultMessage:
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
		return nil, fmt.Errorf("gemini: unknown message type %T", m)
	}
}

func encodeTools(defs []tool.Definition) []map[string]any {
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

func mapFinishReason(reason string) llm.StopReason {
	switch reason {
	case "STOP":
		return llm.StopEndTurn
	case "MAX_TOKENS":
		return llm.StopMaxTokens
	case "SAFETY":
		return llm.StopContentFilter
	case "RECITATION":
		return llm.StopContentFilter
	case "TOOL_USE":
		return llm.StopToolUse
	default:
		return llm.StopEndTurn
	}
}
