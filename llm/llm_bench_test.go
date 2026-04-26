package llm

import (
	"strconv"
	"testing"
)

func registerDialects(b *testing.B, n int) *DialectRegistry {
	b.Helper()
	r := NewDialectRegistry()
	for i := 0; i < n; i++ {
		if err := r.Register("d"+strconv.Itoa(i), &mockDialect{name: "d" + strconv.Itoa(i)}); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
	return r
}

func BenchmarkDialectRegistry_Register(b *testing.B) {
	b.ReportAllocs()
	r := NewDialectRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Register("d"+strconv.Itoa(i), &mockDialect{name: "d" + strconv.Itoa(i)})
	}
}

func BenchmarkDialectRegistry_Get(b *testing.B) {
	const n = 64
	r := registerDialects(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "d" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Get(keys[i&(n-1)]); err != nil {
			b.Fatalf("get: %v", err)
		}
	}
}
