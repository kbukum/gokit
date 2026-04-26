package storage

import (
	"strconv"
	"testing"

	"github.com/kbukum/gokit/logger"
)

func benchFactory(_ Config, _ any, _ *logger.Logger) (Storage, error) {
	return nil, nil
}

func registerStorageFactories(b *testing.B, n int) *FactoryRegistry {
	b.Helper()
	r := NewFactoryRegistry()
	for i := 0; i < n; i++ {
		if err := r.Register("s"+strconv.Itoa(i), benchFactory); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
	return r
}

func BenchmarkFactoryRegistry_Register(b *testing.B) {
	b.ReportAllocs()
	r := NewFactoryRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Register("s"+strconv.Itoa(i), benchFactory)
	}
}

func BenchmarkFactoryRegistry_Get(b *testing.B) {
	const n = 64
	r := registerStorageFactories(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "s" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.Get(keys[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}
