package chain_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/chain"
)

type ctxKey string

type contextAwareOp struct {
	chain.BaseOperation
	id   string
	seen *string
}

func (o *contextAwareOp) ID() string { return o.id }

func (o *contextAwareOp) Execute(ctx context.Context, input any, progress chain.ProgressFn) (any, error) {
	_ = progress
	if value, ok := ctx.Value(ctxKey("trace_id")).(string); ok {
		*o.seen = value
	}
	return input, nil
}

func TestChainPropagatesContext(t *testing.T) {
	t.Parallel()

	var seen string
	executor := chain.NewBuilder().Step(&contextAwareOp{id: "ctx", seen: &seen}).Build()
	ctx := context.WithValue(context.Background(), ctxKey("trace_id"), "trace-123")

	result, err := executor.Execute(ctx, "input", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if seen != "trace-123" {
		t.Fatalf("seen context value = %q, want trace-123", seen)
	}
	if result.FinalOutput != "input" {
		t.Fatalf("final output = %v, want input", result.FinalOutput)
	}
}
