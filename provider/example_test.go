package provider_test

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/provider"
)

// exampleEchoProvider is a tiny demo Provider used by the example.
type exampleEchoProvider struct{ name string }

func (e *exampleEchoProvider) Name() string                       { return e.name }
func (e *exampleEchoProvider) IsAvailable(_ context.Context) bool { return true }

func ExampleNewRegistry() {
	reg := provider.NewRegistry[*exampleEchoProvider]()
	reg.RegisterFactory("echo", func(cfg map[string]any) (*exampleEchoProvider, error) {
		name, _ := cfg["name"].(string)
		return &exampleEchoProvider{name: name}, nil
	})

	p, err := reg.Create("echo", map[string]any{"name": "demo"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(p.Name(), p.IsAvailable(context.Background()))
	// Output: demo true
}
