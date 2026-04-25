package pipeline

import (
	"context"
	"testing"
)

func BenchmarkPipelineMapOperation(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := Map(FromSlice([]int{1, 2, 3, 4, 5}), func(_ context.Context, v int) (int, error) { return v * 2, nil })
		_, err := Collect(ctx, p)
		if err != nil {
			b.Fatalf("collect failed: %v", err)
		}
	}
}
