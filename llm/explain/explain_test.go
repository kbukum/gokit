package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/provider"
)

// mockLLM returns a preset JSON response for testing.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Name() string                       { return "mock-llm" }
func (m *mockLLM) IsAvailable(_ context.Context) bool { return true }
func (m *mockLLM) Execute(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	if m.err != nil {
		return llm.CompletionResponse{}, m.err
	}
	return llm.CompletionResponse{Content: m.response}, nil
}

// Verify mockLLM implements the interface.
var _ provider.RequestResponse[llm.CompletionRequest, llm.CompletionResponse] = (*mockLLM)(nil)

func validExplanationJSON() string {
	exp := Explanation{
		Summary: "Analysis indicates the content is likely AI-generated.",
		Reasoning: []ReasoningStep{
			{Signal: "frequency_score", Finding: "High frequency anomalies detected", Impact: "high"},
			{Signal: "metadata_score", Finding: "Metadata appears normal", Impact: "low"},
		},
		KeyFactors: []string{"frequency_score"},
		Confidence: 0.87,
	}
	b, _ := json.Marshal(exp)
	return string(b)
}

func TestGenerate_Basic(t *testing.T) {
	mock := &mockLLM{response: validExplanationJSON()}

	result, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{
			{Name: "frequency_score", Value: 0.92, Label: "Frequency analysis"},
			{Name: "metadata_score", Value: 0.15, Label: "Metadata check"},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if len(result.Reasoning) != 2 {
		t.Errorf("expected 2 reasoning steps, got %d", len(result.Reasoning))
	}
	if result.Confidence != 0.87 {
		t.Errorf("confidence = %f, want 0.87", result.Confidence)
	}
	if len(result.KeyFactors) != 1 || result.KeyFactors[0] != "frequency_score" {
		t.Errorf("key_factors = %v, want [frequency_score]", result.KeyFactors)
	}
}

func TestGenerate_NoSignals(t *testing.T) {
	mock := &mockLLM{response: "{}"}

	_, err := Generate(context.Background(), mock, Request{})
	if err == nil {
		t.Error("expected error for empty signals")
	}
}

func TestGenerate_LLMError(t *testing.T) {
	mock := &mockLLM{err: fmt.Errorf("connection refused")}

	_, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{{Name: "test", Value: 0.5, Label: "Test"}},
	})
	if err == nil {
		t.Error("expected error when LLM fails")
	}
}

func TestGenerate_MalformedJSON(t *testing.T) {
	mock := &mockLLM{response: "this is not json at all"}

	_, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{{Name: "test", Value: 0.5, Label: "Test"}},
	})
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestGenerate_MarkdownWrappedJSON(t *testing.T) {
	// LLMs sometimes wrap JSON in markdown code fences.
	wrapped := "```json\n" + validExplanationJSON() + "\n```"
	mock := &mockLLM{response: wrapped}

	result, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{
			{Name: "frequency_score", Value: 0.92, Label: "Frequency analysis"},
		},
	})
	if err != nil {
		t.Fatalf("Generate with markdown: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary from markdown-wrapped JSON")
	}
}

func TestGenerate_WithContext(t *testing.T) {
	mock := &mockLLM{response: validExplanationJSON()}

	result, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{
			{Name: "score", Value: 0.5, Label: "Score"},
		},
		Context: "Analyzing a 30-second video clip from social media.",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestGenerate_CustomTemplate(t *testing.T) {
	mock := &mockLLM{response: validExplanationJSON()}

	customTmpl := `Analyze these signals:
{{range .Signals}}- {{.Label}}: {{.Value}}
{{end}}
Respond with JSON only.`

	result, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{
			{Name: "test", Value: 0.75, Label: "Test signal"},
		},
		Template: customTmpl,
	})
	if err != nil {
		t.Fatalf("Generate with custom template: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestGenerate_InvalidTemplate(t *testing.T) {
	mock := &mockLLM{response: "{}"}

	_, err := Generate(context.Background(), mock, Request{
		Signals:  []Signal{{Name: "test", Value: 0.5, Label: "Test"}},
		Template: "{{.Invalid",
	})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestGenerate_WithMaxTokens(t *testing.T) {
	mock := &mockLLM{response: validExplanationJSON()}

	result, err := Generate(context.Background(), mock, Request{
		Signals: []Signal{
			{Name: "score", Value: 0.5, Label: "Score"},
		},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestRenderTemplate_Default(t *testing.T) {
	prompt, err := renderTemplate(Request{
		Signals: []Signal{
			{Name: "freq", Value: 0.92, Label: "Frequency"},
			{Name: "meta", Value: 0.10, Label: "Metadata"},
		},
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	// Should contain signal names.
	if !contains(prompt, "freq") || !contains(prompt, "meta") {
		t.Error("prompt should contain signal names")
	}
	if !contains(prompt, "0.9200") {
		t.Error("prompt should contain formatted signal value")
	}
}

func TestRenderTemplate_WithContext(t *testing.T) {
	prompt, err := renderTemplate(Request{
		Signals: []Signal{{Name: "s", Value: 0.5, Label: "S"}},
		Context: "Video from TikTok",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !contains(prompt, "Video from TikTok") {
		t.Error("prompt should contain the context")
	}
}

func TestReasoningStep_Fields(t *testing.T) {
	step := ReasoningStep{
		Signal:  "test_signal",
		Finding: "Anomalous pattern detected",
		Impact:  "high",
	}
	if step.Signal != "test_signal" {
		t.Error("Signal field mismatch")
	}
	if step.Finding != "Anomalous pattern detected" {
		t.Error("Finding field mismatch")
	}
	if step.Impact != "high" {
		t.Error("Impact field mismatch")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || s != "" && containsSub(s, sub))
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
