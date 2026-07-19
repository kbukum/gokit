package agent

import (
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

func (a *Agent) buildRequest(msgs []chat.Message) llm.CompletionRequest {
	req := llm.CompletionRequest{Messages: msgs}
	if a.config.SystemPromptTemplate != nil {
		if rendered, err := a.config.SystemPromptTemplate.Render(a.config.SystemPromptData); err == nil {
			req.SystemPrompt = rendered
		}
	} else if a.config.SystemPrompt != "" {
		req.SystemPrompt = a.config.SystemPrompt
	}
	if a.config.Model != "" {
		req.Model = a.config.Model
	}
	if a.config.Tools != nil {
		req.Tools = a.config.Tools.ToolSpecs()
	}
	return req
}
