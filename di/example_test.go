package di_test

import (
	"fmt"

	"github.com/kbukum/gokit/di"
)

type greetingService struct{ greeting string }

func (g *greetingService) Greet(name string) string {
	return g.greeting + ", " + name + "!"
}

func ExampleNewContainer() {
	c := di.NewContainer()
	defer func() { _ = c.Close() }()

	_ = c.RegisterEager("greeter", func() *greetingService {
		return &greetingService{greeting: "Hello"}
	})

	svc := di.MustResolve[*greetingService](c, "greeter")
	fmt.Println(svc.Greet("World"))
	// Output: Hello, World!
}

func ExampleResolve() {
	c := di.NewContainer()
	defer func() { _ = c.Close() }()

	_ = c.RegisterSingleton("answer", 42)

	v, err := di.Resolve[int](c, "answer")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(v)
	// Output: 42
}
