package chain

import (
	"context"
	"strconv"
	"testing"
)

type noopOp struct {
	BaseOperation
	id string
}

func (n noopOp) ID() string   { return n.id }
func (n noopOp) Name() string { return n.id }
func (n noopOp) Execute(_ context.Context, input any, _ ProgressFn) (any, error) {
	return input, nil
}

func makeOps(n int) []Operation {
	ops := make([]Operation, n)
	for i := 0; i < n; i++ {
		ops[i] = noopOp{id: "op" + strconv.Itoa(i)}
	}
	return ops
}

func BenchmarkExecutor_Execute(b *testing.B) {
	for _, n := range []int{1, 4, 16, 64} {
		b.Run("ops="+strconv.Itoa(n), func(b *testing.B) {
			exec := NewExecutor(makeOps(n))
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := exec.Execute(ctx, "input", nil); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
	}
}

func BenchmarkBuilder_Build(b *testing.B) {
	ops := makeOps(8)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bd := NewBuilder()
		for _, op := range ops {
			bd = bd.Step(op)
		}
		_ = bd.Build()
	}
}
