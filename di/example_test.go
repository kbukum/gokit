package di_test

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/di"
)

type greetingService struct{ greeting string }

func (g *greetingService) Greet(name string) string {
	return g.greeting + ", " + name + "!"
}

func ExampleRegister() {
	c := di.NewContainer()
	defer func() { _ = c.Close(context.Background()) }()

	_ = di.Register(c, &greetingService{greeting: "Hello"})

	svc := di.MustResolve[*greetingService](context.Background(), c)
	fmt.Println(svc.Greet("World"))
	// Output: Hello, World!
}

func ExampleResolve() {
	c := di.NewContainer()
	defer func() { _ = c.Close(context.Background()) }()

	_ = di.RegisterSingleton(c, func(context.Context) (int, error) { return 42, nil })

	v, err := di.Resolve[int](context.Background(), c)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(v)
	// Output: 42
}
