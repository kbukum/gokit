package registry

import (
	"strconv"
	"testing"
)

// item is a small concrete value type used for the benchmarks; using a struct
// (not a pointer/interface) exercises the registry's nil-detection fast path.
type item struct{ id int }

func makeRegistry(b *testing.B, n int) *Registry[*item] {
	b.Helper()
	r := New[*item]("bench")
	for i := 0; i < n; i++ {
		if err := r.Register("k"+strconv.Itoa(i), &item{id: i}); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
	return r
}

func BenchmarkRegistryRegister(b *testing.B) {
	b.ReportAllocs()
	r := New[*item]("bench")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Register("k"+strconv.Itoa(i), &item{id: i})
	}
}

func BenchmarkRegistryGet_Hit(b *testing.B) {
	const n = 1024
	r := makeRegistry(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.Get(keys[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkRegistryLookup_Hit(b *testing.B) {
	const n = 1024
	r := makeRegistry(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Lookup(keys[i&(n-1)]); err != nil {
			b.Fatalf("lookup: %v", err)
		}
	}
}

func BenchmarkRegistryLookup_Miss(b *testing.B) {
	r := makeRegistry(b, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Lookup("missing"); err == nil {
			b.Fatal("expected miss")
		}
	}
}

func BenchmarkRegistryNames(b *testing.B) {
	r := makeRegistry(b, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Names()
	}
}

func BenchmarkRegistryEach(b *testing.B) {
	r := makeRegistry(b, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Each(func(_ string, _ *item) {})
	}
}
