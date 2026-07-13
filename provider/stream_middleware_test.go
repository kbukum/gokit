package provider_test

import (
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestChainStream_AppliesInOrder(t *testing.T) {
	var order []string
	mw := func(tag string) provider.StreamMiddleware[string, byte] {
		return func(inner provider.Stream[string, byte]) provider.Stream[string, byte] {
			order = append(order, tag)
			return inner
		}
	}
	chained := provider.ChainStream(mw("a"), mw("b"), mw("c"))
	result := chained(&splitProvider{})
	if result == nil {
		t.Fatal("expected non-nil chained stream")
	}
	// Innermost (c) is constructed first, outermost (a) last.
	if len(order) != 3 || order[0] != "c" || order[2] != "a" {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}
