package dag

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// Node is the execution unit in a DAG.
type Node interface {
	Name() string
	Run(ctx context.Context, state *State) (any, error)
}

// NodeConfig configures a provider-backed node.
type NodeConfig[I, O any] struct {
	// Name is the unique node identifier in the graph.
	Name string
	// Service is the provider to execute.
	Service provider.RequestResponse[I, O]
	// Extract reads inputs from state.
	Extract func(state *State) (I, error)
	// Output is the port where the result is written.
	Output Port[O]
}

// FromProvider bridges a provider.RequestResponse[I,O] into a DAG Node.
func FromProvider[I, O any](cfg NodeConfig[I, O]) Node {
	return &providerNode[I, O]{cfg: cfg}
}

type providerNode[I, O any] struct {
	cfg NodeConfig[I, O]
}

func (n *providerNode[I, O]) Name() string { return n.cfg.Name }

func (n *providerNode[I, O]) Run(ctx context.Context, state *State) (any, error) {
	input, err := n.cfg.Extract(state)
	if err != nil {
		return nil, err
	}

	output, err := n.cfg.Service.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	Write(state, n.cfg.Output, output)
	return output, nil
}
