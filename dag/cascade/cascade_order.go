package cascade

import (
	"sort"

	"github.com/kbukum/gokit/provider"
)

// OrderStrategy determines execution priority when multiple nodes within a stage are ready simultaneously and resources are constrained.
type OrderStrategy func(nodes []orderableNode) []orderableNode

type orderableNode struct {
	Name string
	Meta provider.Meta
}

// OrderByCost sorts nodes by the "cost" metadata key, cheapest first.
func OrderByCost() OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		sort.SliceStable(nodes, func(i, j int) bool {
			ci, _ := nodes[i].Meta.Float("cost")
			cj, _ := nodes[j].Meta.Float("cost")
			return ci < cj
		})
		return nodes
	}
}

// OrderByLatency sorts nodes by the "latency_ms" metadata key, fastest first.
func OrderByLatency() OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		sort.SliceStable(nodes, func(i, j int) bool {
			li, _ := nodes[i].Meta.Float("latency_ms")
			lj, _ := nodes[j].Meta.Float("latency_ms")
			return li < lj
		})
		return nodes
	}
}

// WeightedScore sorts nodes by a weighted combination of metadata dimensions. Higher weights mean that dimension matters more. Nodes with lower weighted scores execute first.
func WeightedScore(weights map[string]float64) OrderStrategy {
	return func(nodes []orderableNode) []orderableNode {
		type scored struct {
			node  orderableNode
			score float64
		}
		items := make([]scored, len(nodes))
		for i, n := range nodes {
			var s float64
			for key, w := range weights {
				v, ok := n.Meta.Float(key)
				if ok {
					s += v * w
				}
			}
			items[i] = scored{node: n, score: s}
		}
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].score < items[j].score
		})
		for i, item := range items {
			nodes[i] = item.node
		}
		return nodes
	}
}
