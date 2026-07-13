package provider_test

import (
	"testing"

	"github.com/kbukum/gokit/provider"
)

func TestChainDuplex_AppliesInOrder(t *testing.T) {
	var order []string
	mw := func(tag string) provider.DuplexMiddleware[string, string] {
		return func(inner provider.Duplex[string, string]) provider.Duplex[string, string] {
			order = append(order, tag)
			return inner
		}
	}
	chained := provider.ChainDuplex(mw("a"), mw("b"), mw("c"))
	result := chained(&echoDuplex{})
	if result == nil {
		t.Fatal("expected non-nil chained duplex")
	}
	if len(order) != 3 || order[0] != "c" || order[2] != "a" {
		t.Fatalf("unexpected middleware order: %v", order)
	}
}
