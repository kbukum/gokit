package dag

import (
	"context"
	"strconv"
	"testing"
)

type noopNode struct{ name string }

func (n noopNode) Name() string                                 { return n.name }
func (n noopNode) Run(_ context.Context, _ *State) (any, error) { return n.name, nil }

// makeChainGraph builds a linear chain of n nodes: n0 -> n1 -> ... -> n(n-1).
func makeChainGraph(n int) *Graph {
	g := &Graph{Nodes: make(map[string]Node, n)}
	for i := 0; i < n; i++ {
		name := "n" + strconv.Itoa(i)
		g.Nodes[name] = noopNode{name: name}
		if i > 0 {
			g.Edges = append(g.Edges, Edge{From: "n" + strconv.Itoa(i-1), To: name})
		}
	}
	return g
}

// makeFanGraph builds 1 root with `width` independent leaves (one parallel level).
func makeFanGraph(width int) *Graph {
	g := &Graph{Nodes: make(map[string]Node, width+1)}
	g.Nodes["root"] = noopNode{name: "root"}
	for i := 0; i < width; i++ {
		name := "leaf" + strconv.Itoa(i)
		g.Nodes[name] = noopNode{name: name}
		g.Edges = append(g.Edges, Edge{From: "root", To: name})
	}
	return g
}

func BenchmarkBuildLevels_Chain(b *testing.B) {
	g := makeChainGraph(32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := BuildLevels(g); err != nil {
			b.Fatalf("levels: %v", err)
		}
	}
}

func BenchmarkEngine_ExecuteBatch_Chain(b *testing.B) {
	for _, n := range []int{4, 16, 64} {
		b.Run("nodes="+strconv.Itoa(n), func(b *testing.B) {
			g := makeChainGraph(n)
			eng := &Engine{}
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := eng.ExecuteBatch(ctx, g, NewState()); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
	}
}

func BenchmarkEngine_ExecuteBatch_Fan(b *testing.B) {
	for _, w := range []int{4, 16, 64} {
		b.Run("width="+strconv.Itoa(w), func(b *testing.B) {
			g := makeFanGraph(w)
			eng := &Engine{MaxParallel: 4}
			ctx := context.Background()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := eng.ExecuteBatch(ctx, g, NewState()); err != nil {
					b.Fatalf("execute: %v", err)
				}
			}
		})
	}
}
