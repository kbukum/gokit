package llm

import "testing"

func TestCompletionRequest_Defaults(t *testing.T) {
	req := CompletionRequest{
		Messages: []Message{{Role: "user", Content: "Hello"}},
	}

	if req.Model != "" {
		t.Errorf("Model should be empty by default, got %q", req.Model)
	}
	if req.Temperature != 0 {
		t.Errorf("Temperature should be 0 by default, got %v", req.Temperature)
	}
	if req.Stream {
		t.Error("Stream should be false by default")
	}
}

func TestStreamFormat_Values(t *testing.T) {
	if StreamNDJSON != 0 {
		t.Errorf("StreamNDJSON = %d, want 0", StreamNDJSON)
	}
	if StreamSSE != 1 {
		t.Errorf("StreamSSE = %d, want 1", StreamSSE)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{Dialect: "test"}
	cfg.applyDefaults()

	if cfg.Timeout != 120e9 { // 120 seconds
		t.Errorf("Timeout = %v, want 120s", cfg.Timeout)
	}
	if cfg.Name != "test-llm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-llm")
	}
}

func TestConfig_ApplyDefaults_PreservesExisting(t *testing.T) {
	cfg := Config{
		Name:    "custom",
		Dialect: "test",
		Timeout: 60e9, // 60 seconds
	}
	cfg.applyDefaults()

	if cfg.Name != "custom" {
		t.Errorf("Name = %q, want %q (should be preserved)", cfg.Name, "custom")
	}
	if cfg.Timeout != 60e9 {
		t.Errorf("Timeout = %v, want 60s (should be preserved)", cfg.Timeout)
	}
}

func TestUsage_Fields(t *testing.T) {
	u := Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}
	if u.PromptTokens+u.CompletionTokens != u.TotalTokens {
		t.Errorf("token math: %d + %d != %d", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
	}
}

func TestExtra_Field(t *testing.T) {
	req := CompletionRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
		Extra:    map[string]any{"think": false, "format": "json"},
	}

	if req.Extra["think"] != false {
		t.Error("Extra['think'] should be false")
	}
	if req.Extra["format"] != "json" {
		t.Errorf("Extra['format'] = %v, want json", req.Extra["format"])
	}
}
