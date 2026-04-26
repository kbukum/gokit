package di

import (
	"strconv"
	"testing"
)

type benchSvc struct{ n int }

func newBenchSvc() *benchSvc { return &benchSvc{n: 42} }

func registerN(b *testing.B, c Container, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		if err := c.Register("svc"+strconv.Itoa(i), newBenchSvc); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
}

func BenchmarkContainerRegister(b *testing.B) {
	b.ReportAllocs()
	c := NewContainer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Register("svc"+strconv.Itoa(i), newBenchSvc)
	}
}

func BenchmarkContainerResolve_Cached(b *testing.B) {
	c := NewContainer()
	registerN(b, c, 64)
	// Prime the cache.
	for i := 0; i < 64; i++ {
		if _, err := c.Resolve("svc" + strconv.Itoa(i)); err != nil {
			b.Fatalf("prime: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Resolve("svc" + strconv.Itoa(i&63)); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkResolve_Generic_Cached(b *testing.B) {
	c := NewContainer()
	registerN(b, c, 64)
	for i := 0; i < 64; i++ {
		_, _ = c.Resolve("svc" + strconv.Itoa(i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Resolve[*benchSvc](c, "svc"+strconv.Itoa(i&63)); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}

func BenchmarkMustResolve_Cached(b *testing.B) {
	c := NewContainer()
	registerN(b, c, 64)
	for i := 0; i < 64; i++ {
		_, _ = c.Resolve("svc" + strconv.Itoa(i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MustResolve[*benchSvc](c, "svc"+strconv.Itoa(i&63))
	}
}

func BenchmarkProvide(b *testing.B) {
	b.ReportAllocs()
	c := NewContainer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := NameKey[*benchSvc]("svc" + strconv.Itoa(i))
		if err := Provide(c, k, newBenchSvc); err != nil {
			b.Fatalf("provide: %v", err)
		}
	}
}

func BenchmarkResolveKey_Cached(b *testing.B) {
	c := NewContainer()
	keys := make([]Key[*benchSvc], 64)
	for i := range keys {
		keys[i] = NameKey[*benchSvc]("svc" + strconv.Itoa(i))
		if err := Provide(c, keys[i], newBenchSvc); err != nil {
			b.Fatalf("provide: %v", err)
		}
		// Prime the cache.
		if _, err := ResolveKey(c, keys[i]); err != nil {
			b.Fatalf("prime: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ResolveKey(c, keys[i&63]); err != nil {
			b.Fatalf("resolve: %v", err)
		}
	}
}
