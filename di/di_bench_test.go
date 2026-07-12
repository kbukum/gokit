package di_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/kbukum/gokit/di"
)

type benchSvc struct{ n int }

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	c := di.NewContainer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = di.Register(c, &benchSvc{n: 42}, di.WithName(strconv.Itoa(i)))
	}
}

func BenchmarkResolve_EagerNamed(b *testing.B) {
	ctx := context.Background()
	c := di.NewContainer()
	for i := 0; i < 64; i++ {
		_ = di.Register(c, &benchSvc{n: i}, di.WithName(strconv.Itoa(i)))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := di.Resolve[*benchSvc](ctx, c, di.WithName(strconv.Itoa(i&63))); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkResolve_SingletonCached(b *testing.B) {
	ctx := context.Background()
	c := di.NewContainer()
	_ = di.RegisterSingleton(c, func(context.Context) (*benchSvc, error) { return &benchSvc{n: 1}, nil })
	if _, err := di.Resolve[*benchSvc](ctx, c); err != nil {
		b.Fatalf("prime: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := di.Resolve[*benchSvc](ctx, c); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkMustResolve(b *testing.B) {
	ctx := context.Background()
	c := di.NewContainer()
	_ = di.Register(c, &benchSvc{n: 1})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = di.MustResolve[*benchSvc](ctx, c)
	}
}
