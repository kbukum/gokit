package workload

import (
	"strconv"
	"testing"

	"github.com/kbukum/gokit/logging"
)

func benchFactory(_ Config, _ any, _ *logging.Logger) (Manager, error) {
	return nil, nil //nolint:nilnil // benchmark stub: no error path exercised
}

func registerFactories(b *testing.B, n int) *FactoryRegistry {
	b.Helper()
	r := NewFactoryRegistry()
	for i := 0; i < n; i++ {
		if err := r.Register("f"+strconv.Itoa(i), benchFactory); err != nil {
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
		_ = r.Register("f"+strconv.Itoa(i), benchFactory)
	}
}

func BenchmarkFactoryRegistry_Get(b *testing.B) {
	const n = 64
	r := registerFactories(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "f" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.Get(keys[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}
