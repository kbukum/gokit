package discovery

import (
	"strconv"
	"testing"

	"github.com/kbukum/gokit/logger"
)

func benchProviderFactory(_ Config, _ *logger.Logger) (Registry, Discovery, error) {
	return nil, nil, nil
}

func registerProviders(b *testing.B, n int) *ProviderRegistry {
	b.Helper()
	r := NewProviderRegistry()
	for i := 0; i < n; i++ {
		if err := r.Register("p"+strconv.Itoa(i), benchProviderFactory); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
	return r
}

func BenchmarkProviderRegistry_Register(b *testing.B) {
	b.ReportAllocs()
	r := NewProviderRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Register("p"+strconv.Itoa(i), benchProviderFactory)
	}
}

func BenchmarkProviderRegistry_Get(b *testing.B) {
	const n = 64
	r := registerProviders(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "p" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.Get(keys[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}
