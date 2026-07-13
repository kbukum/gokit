package inference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient/sse"
)

// OAICompatStreamFunc opens a Server-Sent Events stream, POSTing the
// pre-marshaled JSON body to path. The caller owns transport, auth, headers,
// and retries; the returned reader is closed by [OAICompatPredictStream] when
// the stream ends.
type OAICompatStreamFunc func(ctx context.Context, path string, body json.RawMessage) (sse.Reader, error)

// OAICompatPredictStream is a shared implementation of
// [StreamingInference.PredictStream] for adapters that wrap an
// OpenAI-compatible /v1/completions endpoint (vllm, tgi, etc.).
//
// It validates the request up front (a "prompt" Text input and a model name)
// and returns an error before opening the stream when either is missing, so a
// bad request fails fast. Once the stream is open it emits canonical
// [ai.TextDelta] events per chunk, a trailing [ai.UsageDelta] when the server
// reports usage, and terminates on the "[DONE]" sentinel or reader EOF. Parse
// and transport failures surface as a terminal [ai.Error] event.
func OAICompatPredictStream(ctx context.Context, kind string, open OAICompatStreamFunc, req PredictRequest) (<-chan ai.StreamEvent, error) {
	prompt, err := extractPrompt(kind, req)
	if err != nil {
		return nil, err
	}
	modelName := resolveModel(req)
	if modelName == "" {
		return nil, fmt.Errorf("%s: model name is required", kind)
	}

	body := map[string]any{
		"model":  modelName,
		"prompt": prompt,
		"stream": true,
	}
	for k, v := range req.Parameters {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", kind, err)
	}

	reader, err := open(ctx, "/v1/completions", raw)
	if err != nil {
		return nil, fmt.Errorf("%s: open stream: %w", kind, err)
	}

	ch := make(chan ai.StreamEvent, 1)
	go streamOAICompat(ctx, kind, reader, ch)
	return ch, nil
}

// oaiCompatStreamChunk is one /v1/completions SSE data payload.
type oaiCompatStreamChunk struct {
	Choices []struct {
		Text         string `json:"text"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func streamOAICompat(ctx context.Context, kind string, reader sse.Reader, ch chan<- ai.StreamEvent) {
	defer close(ch)
	defer func() { _ = reader.Close() }()

	emit := func(ev ai.StreamEvent) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for {
		if ctx.Err() != nil {
			return
		}
		event, err := reader.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				emit(ai.Error{Err: fmt.Errorf("%s: stream read: %w", kind, err)})
			}
			return
		}

		data := strings.TrimSpace(event.Data)
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return
		}

		var chunk oaiCompatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			emit(ai.Error{Err: fmt.Errorf("%s: parse stream chunk: %w", kind, err)})
			return
		}

		for _, c := range chunk.Choices {
			if c.Text != "" {
				if !emit(ai.TextDelta{Text: c.Text}) {
					return
				}
			}
		}
		if chunk.Usage != nil {
			if !emit(ai.UsageDelta{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}) {
				return
			}
		}
	}
}
