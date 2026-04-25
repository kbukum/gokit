package pipeline_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/pipeline"
)

func ExampleFromSlice() {
	p := pipeline.FromSlice([]int{1, 2, 3, 4})
	doubled := pipeline.Map(p, func(_ context.Context, x int) (int, error) {
		return x * 2, nil
	})
	got, err := pipeline.Collect(context.Background(), doubled)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(got)
	// Output: [2 4 6 8]
}

func ExampleFilter() {
	p := pipeline.FromSlice([]string{"alpha", "", "beta", ""})
	nonEmpty := pipeline.Filter(p, func(s string) bool { return s != "" })
	got, _ := pipeline.Collect(context.Background(), nonEmpty)
	fmt.Println(strings.Join(got, ","))
	// Output: alpha,beta
}
