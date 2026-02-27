package dag

import "fmt"

// Graph declares nodes and edges (dependency relationships).
type Graph struct {
	Nodes map[string]Node
	Edges []Edge
}

// Edge represents a dependency: To depends on From.
type Edge struct {
	From string
	To   string
}

// BuildLevels uses Kahn's algorithm to group nodes by dependency level.
// Nodes within the same level can execute in parallel.
// Returns an error if a cycle is detected.
func BuildLevels(g *Graph) ([][]string, error) {
	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // from -> [to...]

	for name := range g.Nodes {
		inDegree[name] = 0
	}

	for _, e := range g.Edges {
		if _, ok := g.Nodes[e.From]; !ok {
			return nil, fmt.Errorf("dag: edge references unknown node %q", e.From)
		}
		if _, ok := g.Nodes[e.To]; !ok {
			return nil, fmt.Errorf("dag: edge references unknown node %q", e.To)
		}
		inDegree[e.To]++
		dependents[e.From] = append(dependents[e.From], e.To)
	}

	// Collect nodes with no incoming edges (level 0)
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var levels [][]string
	visited := 0

	for len(queue) > 0 {
		levels = append(levels, queue)
		visited += len(queue)

		var next []string
		for _, name := range queue {
			for _, dep := range dependents[name] {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					next = append(next, dep)
				}
			}
		}
		queue = next
	}

	if visited != len(g.Nodes) {
		return nil, fmt.Errorf("dag: cycle detected, processed %d of %d nodes", visited, len(g.Nodes))
	}

	return levels, nil
}
