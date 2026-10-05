package anthropic

import "github.com/kbukum/gokit/llm/internal/streamwire"

type streamUsage struct {
	Input  *int `json:"input_tokens"`
	Output *int `json:"output_tokens"`
	Cached *int `json:"cache_read_input_tokens"`
}

func (u *streamUsage) update() *streamwire.UsageUpdate {
	if u == nil {
		return nil
	}
	return &streamwire.UsageUpdate{InputTokens: u.Input, OutputTokens: u.Output, CachedTokens: u.Cached}
}
