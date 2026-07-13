package dag

import (
	"testing"
)

func TestBuildLevels_LongCycle(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e"}
	nodes := make(map[string]Node, len(names))
	for _, name := range names {
		nodes[name] = newFuncNode(name, nil)
	}

	// a→b→c→d→e→a
	edges := []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "d", To: "e"},
		{From: "e", To: "a"},
	}

	g := &Graph{Nodes: nodes, Edges: edges}
	_, err := BuildLevels(g)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}
