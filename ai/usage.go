package ai

import (
	"errors"
	"time"
)

// Usage reports model token accounting.
type Usage struct {
	InputTokens     int `json:"input_tokens,omitempty"`
	OutputTokens    int `json:"output_tokens,omitempty"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// TotalTokens returns input plus output tokens.
func (u Usage) TotalTokens() int { return u.InputTokens + u.OutputTokens }

// Decimal is a fixed-scale decimal value in nanos of one currency unit.
type Decimal struct {
	Units int64 `json:"units"`
	Nanos int32 `json:"nanos"`
}

// Cost reports cost components for a model call.
type Cost struct {
	Input     Decimal `json:"input"`
	Output    Decimal `json:"output"`
	Cached    Decimal `json:"cached"`
	Reasoning Decimal `json:"reasoning"`
	Currency  string  `json:"currency"`
}

// Budget carries shared budget vocabulary. Enforcement belongs to callers.
type Budget struct {
	MaxTokens int           `json:"max_tokens,omitempty"`
	MaxCalls  int           `json:"max_calls,omitempty"`
	MaxCost   Cost          `json:"max_cost,omitempty"`
	WallClock time.Duration `json:"wall_clock,omitempty"`
}

// BudgetExceededReason identifies which budget was exceeded.
type BudgetExceededReason string

const (
	BudgetExceededTokens    BudgetExceededReason = "tokens"
	BudgetExceededCalls     BudgetExceededReason = "calls"
	BudgetExceededCost      BudgetExceededReason = "cost"
	BudgetExceededWallClock BudgetExceededReason = "wall_clock"
	BudgetExceededCancelled BudgetExceededReason = "cancelled" //nolint:misspell // Contract spelling.
)

var ErrBudgetExceeded = errors.New("ai: budget exceeded")

// BudgetExceededError carries the specific budget reason and matches ErrBudgetExceeded with errors.Is.
type BudgetExceededError struct {
	Reason BudgetExceededReason
}

func (e BudgetExceededError) Error() string {
	if e.Reason == "" {
		return ErrBudgetExceeded.Error()
	}
	return ErrBudgetExceeded.Error() + ": " + string(e.Reason)
}

func (e BudgetExceededError) Is(target error) bool { return target == ErrBudgetExceeded }
