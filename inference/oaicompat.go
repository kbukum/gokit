package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kbukum/gokit/ai"
)

// OAICompatExecuteFunc is the minimal signature for the HTTP layer the OAI-compat helpers use.
// It returns the response body and a non-nil error on transport / status failures.
type OAICompatExecuteFunc func(ctx context.Context, method, path string, body any) ([]byte, error)

// OAICompatPredict is a shared implementation of [Inference.Predict] for adapters that wrap an OpenAI-compatible /v1/completions endpoint (vllm, tgi, etc.).
// Per locked decision D4, both expose OpenAI-compat endpoints;
// each thin adapter is ~50 LOC of glue around this helper.
//
// It expects a "prompt" Text input and returns a "text" Text output.
//
//   - kind: adapter kind (e.g. "vllm", "tgi") — used in errors and Model.
//   - exec: HTTP execute function (caller owns auth, headers, retries).
func OAICompatPredict(ctx context.Context, kind string, exec OAICompatExecuteFunc, req PredictRequest) (PredictResponse, error) {
	prompt, err := extractPrompt(kind, req)
	if err != nil {
		return PredictResponse{}, err
	}
	modelName := resolveModel(req)
	if modelName == "" {
		return PredictResponse{}, fmt.Errorf("%s: model name is required", kind)
	}
	body := map[string]any{
		"model":  modelName,
		"prompt": prompt,
	}
	for k, v := range req.Parameters {
		body[k] = v
	}

	respBody, err := exec(ctx, http.MethodPost, "/v1/completions", body)
	if err != nil {
		return PredictResponse{}, fmt.Errorf("%s: completions request failed: %w", kind, err)
	}

	var raw struct {
		Choices []struct {
			Text         string `json:"text"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return PredictResponse{}, fmt.Errorf("%s: parse response: %w", kind, err)
	}
	if len(raw.Choices) == 0 {
		return PredictResponse{}, fmt.Errorf("%s: response has no choices", kind)
	}

	finishReason := strings.TrimSpace(raw.Choices[0].FinishReason)
	metadata := map[string]string{}
	if finishReason != "" {
		metadata["finish_reason"] = finishReason
	}
	return PredictResponse{
		Outputs: map[string]Value{
			"text": TextValue(raw.Choices[0].Text),
		},
		Model:  ai.Model{Name: modelName, Provider: ai.ProviderCustom},
		Status: StatusSuccess,
		Usage: Usage{
			InputTokens:  raw.Usage.PromptTokens,
			OutputTokens: raw.Usage.CompletionTokens,
		},
		Metadata: metadata,
	}, nil
}

func extractPrompt(kind string, req PredictRequest) (string, error) {
	if v, ok := req.Inputs["prompt"]; ok {
		if v.Kind != KindText {
			return "", fmt.Errorf(`%s: input "prompt" must be Text (got %s)`, kind, v.Kind)
		}
		return v.Text, nil
	}
	if v, ok := req.Inputs["text"]; ok && v.Kind == KindText {
		return v.Text, nil
	}
	return "", fmt.Errorf(`%s: missing required input "prompt"`, kind)
}

func resolveModel(req PredictRequest) string {
	if name := strings.TrimSpace(req.ModelName); name != "" {
		return name
	}
	return ""
}
