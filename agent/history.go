package agent

import (
	"context"

	"github.com/kbukum/gokit/ai/chat"
)

func (a *Agent) contextTooLarge(msgs []chat.Message) bool {
	caps := a.config.Provider.Capabilities()
	return caps.MaxInputTokens > 0 && a.config.Provider.CountTokens(msgs) > caps.MaxInputTokens
}

func (a *Agent) persistHistory(ctx context.Context, msgs []chat.Message) {
	if a.config.Store != nil && a.config.SessionID != "" {
		_ = a.config.Store.Save(ctx, a.config.SessionID, msgs)
	}
}
