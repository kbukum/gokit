package tool

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
)

type benchInput struct {
	X int `json:"x"`
}

type benchOutput struct {
	Y int `json:"y"`
}

func benchHandler(_ context.Context, in benchInput) (benchOutput, error) {
	return benchOutput{Y: in.X + 1}, nil
}

func registerTools(b *testing.B, n int) *Registry {
	b.Helper()
	r := NewRegistry()
	for i := 0; i < n; i++ {
		c := FromFunc("t"+strconv.Itoa(i), "bench tool", benchHandler).AsCallable()
		if err := r.Register(c); err != nil {
			b.Fatalf("register: %v", err)
		}
	}
	return r
}

func BenchmarkRegistry_Register(b *testing.B) {
	b.ReportAllocs()
	r := NewRegistry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := FromFunc("t"+strconv.Itoa(i), "bench tool", benchHandler).AsCallable()
		_ = r.Register(c)
	}
}

func BenchmarkRegistry_Get_Hit(b *testing.B) {
	const n = 256
	r := registerTools(b, n)
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "t" + strconv.Itoa(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := r.Get(keys[i&(n-1)]); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkRegistry_Call(b *testing.B) {
	r := registerTools(b, 4)
	tctx := Background()
	input := json.RawMessage(`{"x":1}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Call(tctx, "t0", input); err != nil {
			b.Fatalf("call: %v", err)
		}
	}
}
