package agent

import (
	"context"

	"github.com/kbukum/gokit/ai/chat"
)

func (a *Agent) contextTooLarge(msgs []chat.Message) bool {
	budget := a.config.Provider.Capabilities().InputBudget()
	return budget > 0 && a.config.Provider.CountTokens(msgs) > budget
}

func (a *Agent) persistHistory(ctx context.Context, msgs []chat.Message) {
	if a.config.Store != nil && a.config.SessionID != "" {
		_ = a.config.Store.Save(ctx, a.config.SessionID, msgs)
	}
}
