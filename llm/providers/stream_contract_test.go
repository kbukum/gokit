package providers_test

import (
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/anthropic"
	"github.com/kbukum/gokit/llm/providers/gemini"
	"github.com/kbukum/gokit/llm/providers/openai"
)

func TestStreamObservations(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		dialect                   llm.Dialect
		reasoning, usage, failure string
	}{
		{"openai", &openai.Dialect{}, `{"choices":[{"delta":{"reasoning_content":"think"}}]}`, `{"choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0}}`, `{"error":{"message":"private upstream details"}}`},
		{"anthropic", &anthropic.Dialect{}, `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"think"}}`, `{"type":"message_delta","usage":{"output_tokens":0}}`, `{"type":"error","error":{"type":"overloaded_error","message":"private upstream details"}}`},
		{"gemini", &gemini.Dialect{}, `{"candidates":[{"content":{"parts":[{"text":"think","thought":true}]}}]}`, `{"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`, `{"error":{"code":500,"message":"private upstream details"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := tc.dialect.ParseStreamChunk([]byte(tc.reasoning))
			if err != nil || chunk.Reasoning != "think" || chunk.Content != "" {
				t.Fatalf("reasoning=%+v err=%v", chunk, err)
			}
			chunk, err = tc.dialect.ParseStreamChunk([]byte(tc.usage))
			if err != nil || chunk.Usage == nil {
				t.Fatalf("zero usage lost: %+v %v", chunk, err)
			}
			var usage ai.Usage
			if err := chunk.Usage.Apply(&usage); err != nil || usage.TotalTokens() != 0 {
				t.Fatalf("usage=%+v err=%v", usage, err)
			}
			if _, err := tc.dialect.ParseStreamChunk([]byte(tc.failure)); err == nil {
				t.Fatal("provider failure treated as success")
			}
		})
	}
	d := &openai.Dialect{}
	finish, err := d.ParseStreamChunk([]byte(`{"choices":[{"delta":{},"finish_reason":"length"}]}`))
	if err != nil || finish.Done || finish.StopReason != "length" {
		t.Fatalf("finish reason ended usage tail: %+v %v", finish, err)
	}
	done, err := d.ParseStreamChunk([]byte("[DONE]"))
	if err != nil || !done.Done {
		t.Fatalf("terminal marker: %+v %v", done, err)
	}
}
