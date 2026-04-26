package tool_test

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/tool"
)

// AddInput is a tiny demo input type used by the example.
type AddInput struct {
	A int `json:"a"`
	B int `json:"b"`
}

// AddOutput is the matching output type.
type AddOutput struct {
	Sum int `json:"sum"`
}

// ExampleFromFunc shows the most common way to build a typed tool from a plain
// Go function. Schema generation and JSON (de)serialisation are handled by
// FromFunc — your function works in real Go types.
func ExampleFromFunc() {
	add := tool.FromFunc("add", "Add two integers",
		func(_ context.Context, in AddInput) (AddOutput, error) {
			return AddOutput{Sum: in.A + in.B}, nil
		},
	)

	out, err := add.Execute(context.Background(), AddInput{A: 2, B: 3})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(out.Sum)
	// Output: 5
}

// ExampleNewRegistry shows how to register tools into a registry, then look
// them up by name (the typical pattern for agent / MCP integration).
func ExampleNewRegistry() {
	add := tool.FromFunc("add", "Add two integers",
		func(_ context.Context, in AddInput) (AddOutput, error) {
			return AddOutput{Sum: in.A + in.B}, nil
		},
	)

	reg := tool.NewRegistry()
	if err := reg.Register(add.AsCallable()); err != nil {
		fmt.Println("register:", err)
		return
	}

	// Retrieve by name and execute through the Callable interface.
	got, ok := reg.Get("add")
	fmt.Println("registered:", ok, "name:", got.Definition().Name, "tools:", reg.Len())
	// Output: registered: true name: add tools: 1
}
