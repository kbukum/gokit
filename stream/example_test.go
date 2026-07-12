package stream_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/kbukum/gokit/stream"
)

func ExampleFromSlice() {
	p := stream.FromSlice([]int{1, 2, 3, 4})
	doubled := stream.Map(p, func(_ context.Context, x int) (int, error) {
		return x * 2, nil
	})
	got, err := stream.Collect(context.Background(), doubled)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(got)
	// Output: [2 4 6 8]
}

func ExampleFilter() {
	p := stream.FromSlice([]string{"alpha", "", "beta", ""})
	nonEmpty := stream.Filter(p, func(s string) bool { return s != "" })
	got, _ := stream.Collect(context.Background(), nonEmpty)
	fmt.Println(strings.Join(got, ","))
	// Output: alpha,beta
}
