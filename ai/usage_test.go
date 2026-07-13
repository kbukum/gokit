package ai_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/ai"
)

func TestBudgetExceededError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		reason ai.BudgetExceededReason
		want   string
	}{
		{"no reason", "", "ai: budget exceeded"},
		{"token reason", ai.BudgetExceededTokens, "ai: budget exceeded: tokens"},
		{"cancelled reason", ai.BudgetExceededCancelled, "ai: budget exceeded: cancelled"}, //nolint:misspell // matches BudgetExceededCancelled contract spelling
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ai.BudgetExceededError{Reason: tt.reason}
			if err.Error() != tt.want {
				t.Errorf("Error() = %q, want %q", err.Error(), tt.want)
			}
			if !errors.Is(err, ai.ErrBudgetExceeded) {
				t.Errorf("errors.Is(%v, ErrBudgetExceeded) = false", err)
			}
		})
	}
}
